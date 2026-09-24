package service

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go-admin/internal/dao"
	"go-admin/internal/domain/model"
	"go-admin/internal/dto"
	"go-admin/pkg/core"

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
		core.Metrics.PushDelivered.Add(1)
	default:
		// 慢客户端：通道已满，标记丢弃指标
		core.Metrics.PushDroppedSlow.Add(1)
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
			core.Metrics.OnlineConnections.Add(1)
		}

		if h.Presence != nil {
			_ = h.Presence.RegisterOnline(h.Ctx, uid)
		}
		if h.GroupCache != nil {
			_ = h.GroupCache.AddUserOnlineGroups(h.Ctx, uid)
		}
		core.Z().Infof("user %d connected", uid)
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
		core.Metrics.OnlineConnections.Add(-1)

		if h.Presence != nil {
			_ = h.Presence.UnregisterOnline(h.Ctx, uid)
		}
		if h.GroupCache != nil {
			_ = h.GroupCache.RemoveUserOnlineGroups(h.Ctx, uid)
		}
		core.Z().Infof("user %d disconnected", uid)
	}()
}

// 但websocket客户端发送消息太慢， 主动断开
func (h *WSHub) dropSlowClient(client *WSClient) {
	core.Z().Warnf("drop slow websocket client user_id=%d", client.UserID)
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
	if _, err := c.Hub.MessageRepo.FindUserByID(toUID); err != nil {
		c.sendWSError("user not found")
		return
	}

	message := &model.Message{
		FromUID: c.UserID,
		ToUID:   toUID,
		Content: content,
		IsRead:  false,
	}
	// 同步落库拿到 id，再按前端契约推 data 包裹的标准帧（与 HTTP 路径一致）
	if err := c.Hub.MessageRepo.CreateMessage(message); err != nil {
		core.Z().Errorf("save message failed: %v", err)
		c.sendWSError("send message failed")
		return
	}

	now := time.Now().Format(time.RFC3339)
	receiverMsg, _ := marshalWithPool(map[string]interface{}{
		"type": "message",
		"data": map[string]interface{}{
			"id":         message.ID,
			"from_uid":   c.UserID,
			"to_uid":     toUID,
			"content":    content,
			"created_at": now,
		},
	})
	c.Hub.SendMessageToUser(toUID, receiverMsg)

	confirmMsg, _ := marshalWithPool(map[string]interface{}{
		"type": "message_sent",
		"data": map[string]interface{}{
			"id":         message.ID,
			"to_uid":     toUID,
			"content":    content,
			"created_at": now,
		},
	})
	c.Hub.SendMessageToUser(c.UserID, confirmMsg)
}

func (c *WSClient) handleGroupMessage(groupID uint, content string) {
	content = strings.TrimSpace(content)
	if groupID == 0 || content == "" {
		c.sendWSError("invalid group message")
		return
	}

	group, err := c.Hub.GroupRepo.FindGroupByID(groupID)
	if err != nil {
		c.sendWSError("group not found")
		return
	}
	if group.Status != model.ChatGroupStatusNormal {
		c.sendWSError("group is not available")
		return
	}

	member, err := c.Hub.GroupRepo.FindMember(groupID, c.UserID)
	if err != nil || member.Status != model.ChatGroupMemberStatusActive {
		c.sendWSError("not a group member")
		return
	}

	message := &model.ChatGroupMessage{
		GroupID:   groupID,
		SenderUID: c.UserID,
		Content:   content,
	}
	event := dto.GroupMessageCreatedEvent{
		Type:      "group_message_created",
		GroupID:   groupID,
		FromUID:   c.UserID,
		Content:   content,
		CreatedAt: time.Now(),
	}
	if err := c.Hub.GroupRepo.CreateGroupMessageWithOutboxBuilder(message, func(saved *model.ChatGroupMessage) (*model.MessageOutbox, error) {
		event.MessageID = saved.ID
		event.CreatedAt = saved.CreatedAt
		return buildGroupMessageOutbox(event)
	}); err != nil {
		c.sendWSError("send group message failed")
		return
	}

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
