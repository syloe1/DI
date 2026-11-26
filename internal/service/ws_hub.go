package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"di/internal/dao"
	"di/internal/dto"
	"di/internal/telemetry"
	"di/internal/transport"
	"github.com/gorilla/websocket"
)

type WSClient struct {
	Hub    *WSHub
	Conn   *websocket.Conn
	UserID uint
	Send   chan []byte
	closed uint32 //原子标记 0 为关闭， 1 已关闭
}

type WSHub struct {
	Clients    map[uint]*WSClient
	Register   chan *WSClient
	Unregister chan *WSClient
	Mutex      sync.RWMutex

	MessageRepo dao.MessageRepository
	GroupRepo   dao.GroupRepository
	Publisher   GroupMessagePublisher
	Presence    *PresenceService
	GroupCache  *GroupCacheService
	Cache       dao.UserCache
	Ctx         context.Context

	// MessageService / GroupService：统一 WS 与 HTTP 发送链路。
	// WS 发消息委托给同一套 service 逻辑（防抖/自检/2000 字长校验），不在 hub 里重复业务。
	// 由 container 通过 SetServices 注入（绕开与 WSService 的构造期循环依赖）。
	MessageService *MessageService
	GroupService   *GroupService
}

func NewWSHub(messageRepo dao.MessageRepository, groupRepo dao.GroupRepository, publisher GroupMessagePublisher, presence *PresenceService, groupCache *GroupCacheService, cache dao.UserCache, ctx context.Context) *WSHub {
	return &WSHub{
		Clients:     make(map[uint]*WSClient),
		Register:    make(chan *WSClient),
		Unregister:  make(chan *WSClient),
		MessageRepo: messageRepo,
		GroupRepo:   groupRepo,
		Publisher:   publisher,
		Presence:    presence,
		GroupCache:  groupCache,
		Cache:       cache,
		Ctx:         ctx,
	}
}

// SetServices 注入消息/群服务（统一 WS/HTTP 发送链路）。
// 在 container 里所有 service 创建完成后调用一次，早于任何 WS 连接建立，无并发写。
func (h *WSHub) SetServices(messageService *MessageService, groupService *GroupService) {
	h.MessageService = messageService
	h.GroupService = groupService
}

func (h *WSHub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.registerClient(client)
		case client := <-h.Unregister:
			h.unregisterClient(client)
		}
	}
}

func (h *WSHub) SendMessageToUser(userID uint, message []byte) {
	h.Mutex.RLock()
	client, ok := h.Clients[userID]
	h.Mutex.RUnlock()
	if !ok {
		return
	}

	select {
	case client.Send <- message:
		telemetry.Metrics.PushDelivered.Add(1)
	default:
		// 慢客户端：通道已满，标记丢弃指标
		telemetry.Metrics.PushDroppedSlow.Add(1)
		// 不直接调用 unregisterClient（会获取写锁，与当前可能持有的读锁死锁），
		// 改为异步投递到 Unregister 通道，让 Hub 事件循环安全处理。
		select {
		case h.Unregister <- client:
		default:
			// Unregister 通道也满了，启动 goroutine 保证不丢
			go func() { h.Unregister <- client }()
		}
	}
}

func (h *WSHub) GetOnlineUserSet() map[uint]struct{} {
	h.Mutex.RLock()
	defer h.Mutex.RUnlock()

	users := make(map[uint]struct{}, len(h.Clients))
	for userID := range h.Clients {
		users[userID] = struct{}{}
	}
	return users
}

func (h *WSHub) Shutdown(ctx context.Context) {
	h.Mutex.Lock()
	clients := make([]*WSClient, 0, len(h.Clients))
	for _, client := range h.Clients {
		clients = append(clients, client)
	}
	h.Clients = make(map[uint]*WSClient)
	h.Mutex.Unlock()

	for _, client := range clients {
		close(client.Send)
		_ = client.Conn.Close()
	}

	if h.Presence != nil {
		_ = h.Presence.Shutdown(ctx)
	}
}

// 串行跑IO, 整个hub卡住
// Hub事件循环只做内存map + 锁， 外部IO 丢给goroutine
func (h *WSHub) registerClient(client *WSClient) {
	isNewUser := false
	h.Mutex.Lock()
	old, ok := h.Clients[client.UserID]
	if !ok {
		isNewUser = true
	}
	if ok && old != client {
		// release old resource
		close(old.Send)
		_ = old.Conn.Close()
	}
	h.Clients[client.UserID] = client
	h.Mutex.Unlock()

	// 所有IO异步， 不阻塞Hub主循环
	uid := client.UserID
	go func() {
		if isNewUser {
			telemetry.Metrics.OnlineConnections.Add(1)
		}

		if h.Presence != nil {
			if err := h.Presence.RegisterOnline(h.Ctx, uid); err != nil {
				telemetry.Z().Warnf("register online failed user_id=%d: %v", uid, err)
			}
		}
		if h.GroupCache != nil {
			if err := h.GroupCache.AddUserOnlineGroups(h.Ctx, uid); err != nil {
				telemetry.Z().Warnf("add user online groups failed user_id=%d: %v", uid, err)
			}
		}
		telemetry.Z().Infof("user %d connected", uid)
	}()
}

func (h *WSHub) unregisterClient(client *WSClient) {
	removed := false
	h.Mutex.Lock()
	if current, ok := h.Clients[client.UserID]; ok && current == client {
		close(client.Send)
		_ = client.Conn.Close()
		delete(h.Clients, client.UserID)
		removed = true
	}
	h.Mutex.Unlock()

	if !removed {
		return
	}

	// IO 异步
	uid := client.UserID
	go func() {
		telemetry.Metrics.OnlineConnections.Add(-1)

		if h.Presence != nil {
			if err := h.Presence.UnregisterOnline(h.Ctx, uid); err != nil {
				telemetry.Z().Warnf("unregister online failed user_id=%d: %v", uid, err)
			}
		}
		if h.GroupCache != nil {
			if err := h.GroupCache.RemoveUserOnlineGroups(h.Ctx, uid); err != nil {
				telemetry.Z().Warnf("remove user online groups failed user_id=%d: %v", uid, err)
			}
		}
		telemetry.Z().Infof("user %d disconnected", uid)
	}()
}

// 但websocket客户端发送消息太慢， 主动断开
func (h *WSHub) dropSlowClient(client *WSClient) {
	telemetry.Z().Warnf("drop slow websocket client user_id=%d", client.UserID)
	h.unregisterClient(client)
}

func (c *WSClient) writePump() {
	defer func() {
		if atomic.CompareAndSwapUint32(&c.closed, 0, 1) {
			c.Hub.Unregister <- c
			_ = c.Conn.Close()
		}
	}()

	for {
		select {
		case message, ok := <-c.Send:
			if !ok {
				_ = c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		}
	}
}

func (c *WSClient) readPump() {
	defer func() {
		if atomic.CompareAndSwapUint32(&c.closed, 0, 1) {
			c.Hub.Unregister <- c
			_ = c.Conn.Close()
		}

	}()

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			return
		}
		// 客户端用 JSON 文本帧 {type:"ping"} 保活（非协议层 Ping），
		// 每收到任意帧都要重置读超时，否则 60s 后误断连
		_ = c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		var msgData dto.WSInboundMessage
		if err := json.Unmarshal(message, &msgData); err != nil {
			continue
		}

		switch msgData.Type {
		case "message":
			c.handleMessage(msgData.ToUID, msgData.Content)
		case "group_message":
			c.handleGroupMessage(msgData.GroupID, msgData.Content)
		case "ping":
			c.handlePing()
		}
	}
}

func (c *WSClient) handleMessage(toUID uint, content string) {
	// 统一走 MessageService.SendMessage：目标存在 / 不能发自己 / 防抖与 HTTP 一致。
	// 落库后由 onPrivateMessage 回调推送 message 事件给收件人；这里只补发送确认帧。
	msgSvc := c.Hub.MessageService
	if msgSvc == nil {
		c.sendWSError("send message failed")
		return
	}

	result, err := msgSvc.SendMessage(c.Hub.Ctx, c.UserID, toUID, content)
	if err != nil {
		telemetry.Z().With(
			"user_id", c.UserID,
			"to_uid", toUID,
		).Errorf("ws send private message failed: %v", err)
		c.sendWSErrorFromError(err, "send message failed")
		return
	}

	confirmMsg, _ := marshalWithPool(map[string]interface{}{
		"type": "message_sent",
		"data": map[string]interface{}{
			"id":         result.MessageID,
			"to_uid":     toUID,
			"content":    content,
			"created_at": result.CreatedAt.Format(time.RFC3339),
		},
	})
	c.Hub.SendMessageToUser(c.UserID, confirmMsg)
}

func (c *WSClient) handleGroupMessage(groupID uint, content string) {
	// 统一走 GroupService.SendGroupMessage：2000 字长/群状态/active 成员校验与 HTTP 一致，
	// 落库 + outbox，消费端广播给在线成员（含发送者），这里不重复业务逻辑、也不额外直接推送。
	grpSvc := c.Hub.GroupService
	if grpSvc == nil {
		c.sendWSError("send group message failed")
		return
	}

	if _, err := grpSvc.SendGroupMessage(c.Hub.Ctx, c.UserID, groupID, content, ""); err != nil {
		telemetry.Z().With(
			"user_id", c.UserID,
			"group_id", groupID,
		).Errorf("ws send group message failed: %v", err)
		c.sendWSErrorFromError(err, "send group message failed")
		return
	}

	// 保持原 WS 行为：发送后刷新 active 成员缓存（HTTP 路径无此动作）
	if c.Hub.GroupCache != nil {
		_ = c.Hub.GroupCache.RefreshActiveMembers(c.Hub.Ctx, groupID)
	}
}

func (c *WSClient) handlePing() {
	if c.Hub.Presence != nil {
		_ = c.Hub.Presence.RefreshOnline(c.Hub.Ctx, c.UserID)
	}
	pongMsg, _ := marshalWithPool(dto.WSOutboundMessage{
		Type: "pong",
		Time: time.Now().Format(time.RFC3339),
	})
	c.Hub.SendMessageToUser(c.UserID, pongMsg)
}

func (c *WSClient) sendWSError(message string) {
	errorMsg, _ := marshalWithPool(dto.WSOutboundMessage{
		Type:    "error",
		Message: message,
		Time:    time.Now().Format(time.RFC3339),
	})
	c.Hub.SendMessageToUser(c.UserID, errorMsg)
}

// sendWSErrorFromError 把 service 返回的 BizError 转成 WS 错误帧（保留现有 error 帧格式）。
// 非业务错误（内部 500）用 fallback 文案，细节已在调用处带上下文字段入日志。
func (c *WSClient) sendWSErrorFromError(err error, fallback string) {
	if err == nil {
		return
	}
	var bizErr *transport.BizError
	if errors.As(err, &bizErr) && bizErr.Msg != "" {
		c.sendWSError(bizErr.Msg)
		return
	}
	c.sendWSError(fallback)
}
