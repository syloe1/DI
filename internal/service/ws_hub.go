package service

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"go-admin/internal/dao"
	"go-admin/internal/domain/model"
	"go-admin/internal/dto"

	"github.com/gorilla/websocket"
)

type WSClient struct {
	Hub    *WSHub
	Conn   *websocket.Conn
	UserID uint
	Send   chan []byte
}

type WSHub struct {
	Clients     map[uint]*WSClient
	Register    chan *WSClient
	Unregister  chan *WSClient
	Mutex       sync.RWMutex
	MessageRepo dao.MessageRepository
	GroupRepo   dao.GroupRepository
	Publisher   GroupMessagePublisher
	Presence    *PresenceService
	Cache       dao.UserCache
	Ctx         context.Context
}

func NewWSHub(messageRepo dao.MessageRepository, groupRepo dao.GroupRepository, publisher GroupMessagePublisher, presence *PresenceService, cache dao.UserCache, ctx context.Context) *WSHub {
	return &WSHub{
		Clients:     make(map[uint]*WSClient),
		Register:    make(chan *WSClient),
		Unregister:  make(chan *WSClient),
		MessageRepo: messageRepo,
		GroupRepo:   groupRepo,
		Publisher:   publisher,
		Presence:    presence, //在线状态服务
		Cache:       cache,
		Ctx:         ctx,
	}
}

func (h *WSHub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.Mutex.Lock()
			//重复登录
			//单个用户ID不能同时登录多端
			//新连接上来->自动踢掉旧连接
			if old, ok := h.Clients[client.UserID]; ok {
				close(old.Send)      //关闭go通道
				_ = old.Conn.Close() //关闭websocket网络连接
			}
			h.Clients[client.UserID] = client
			h.Mutex.Unlock()
			//判断redis缓存是否存在
			if h.Cache != nil {
				_ = h.Cache.SAdd(h.Ctx, OnlineUsersKey, client.UserID)
			}
			if h.Presence != nil {
				_ = h.Presence.RegisterOnline(h.Ctx, client.UserID)
			}
			log.Printf("user %d connected", client.UserID)
		case client := <-h.Unregister:
			h.Mutex.Lock()
			if c, ok := h.Clients[client.UserID]; ok && c == client {
				close(client.Send)      //关闭channel
				_ = client.Conn.Close() //关闭websocket网络连接
				delete(h.Clients, client.UserID)
			}
			h.Mutex.Unlock()
			if h.Cache != nil {
				_ = h.Cache.SRem(h.Ctx, OnlineUsersKey, client.UserID)
			}
			if h.Presence != nil {
				_ = h.Presence.UnregisterOnline(h.Ctx, client.UserID)
			}
			log.Printf("user %d disconnected", client.UserID)
		}
	}
}

func (h *WSHub) SendMessageToUser(userID uint, message []byte) {
	h.Mutex.RLock()
	client, ok := h.Clients[userID]
	h.Mutex.RUnlock()

	if ok {
		select {
		case client.Send <- message:
		default:
			close(client.Send)
			h.Mutex.Lock()
			delete(h.Clients, userID)
			h.Mutex.Unlock()
		}
	}
}

func (h *WSHub) GetOnlineUserSet() map[uint]struct{} {
	h.Mutex.RLock()
	defer h.Mutex.RUnlock()

	users := make(map[uint]struct{}, len(h.Clients))
	//主要Key
	for userID := range h.Clients {
		//struct{}{}go最小， 最轻， 不占内存的空结构体
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
	//清空在线列表
	h.Clients = make(map[uint]*WSClient)
	h.Mutex.Unlock()

	for _, client := range clients {
		close(client.Send)
		_ = client.Conn.Close()
	}
	//清理Redis在线状态
	if h.Presence != nil {
		_ = h.Presence.Shutdown(ctx)
	}
}

/*
writePump 启动

	↓

无限等待 c.Send 通道里的消息

	↓

消息来了 → 发给前端

	↓

发送失败 / 通道关闭 → 关闭连接 → 退出
*/
func (c *WSClient) writePump() {
	defer func() {
		_ = c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			if !ok {
				//发关闭帧，优雅带你看Websocket退出循环
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
		c.Hub.Unregister <- c
		_ = c.Conn.Close()
	}()

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			break
		}

		var msgData dto.WSInboundMessage
		//Json->Go结构体
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

/*
前端发单聊消息

	↓

readPump 收到

	↓

handleMessage 处理

	↓

1. 检查对方是否存在
2. 消息存库（永不丢失）
3. 推送给对方（在线立即收）
4. 告诉发送者：发送成功
*/
func (c *WSClient) handleMessage(toUID uint, content string) {
	_, err := c.Hub.MessageRepo.FindUserByID(toUID)
	if err != nil {
		c.sendWSError("user not found")
		return
	}

	message := &model.Message{
		FromUID: c.UserID,
		ToUID:   toUID,
		Content: content,
		IsRead:  false,
	}
	if err := c.Hub.MessageRepo.CreateMessage(message); err != nil {
		log.Printf("save message failed: %v", err)
		return
	}

	now := time.Now().Format(time.RFC3339)
	receiverMsg, _ := json.Marshal(dto.WSOutboundMessage{
		Type:    "message",
		FromUID: c.UserID,
		Content: content,
		Time:    now,
	})
	//Websocket发给对方
	c.Hub.SendMessageToUser(toUID, receiverMsg)
	//给发送着一个 “发送成功”
	confirmMsg, _ := json.Marshal(dto.WSOutboundMessage{
		Type:    "message_sent",
		ToUID:   toUID,
		Content: content,
		Time:    now,
	})
	c.Send <- confirmMsg
}

// 前端发群消息
// ↓
// readPump 接收
// ↓
// handleGroupMessage
// ↓
// 1. 校验群、校验群成员（权限判断）
// 2. 消息存库（永不丢）
// 3. 发消息到 MQ
// 4. 消费者从 MQ 取出 → 群发所有成员
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
	if err := c.Hub.GroupRepo.CreateGroupMessage(message); err != nil {
		c.sendWSError("send group message failed")
		return
	}

	if c.Hub.Publisher == nil {
		log.Printf("group message publisher is nil")
		return
	}
	//异步群发
	//把消息给MQ, 让消费者异步推送给所有群成员
	if err := c.Hub.Publisher.PublishGroupMessageCreated(c.Hub.Ctx, dto.GroupMessageCreatedEvent{
		Type:      "group_message_created",
		MessageID: message.ID,
		GroupID:   groupID,
		FromUID:   c.UserID,
		Content:   content,
		CreatedAt: message.CreatedAt,
	}); err != nil {
		log.Printf("publish group message event failed: %v", err)
	}
}

func (c *WSClient) handlePing() {
	//刷新Redis在线状态
	if c.Hub.Presence != nil {
		_ = c.Hub.Presence.RefreshOnline(c.Hub.Ctx, c.UserID)
	}
	pongMsg, _ := json.Marshal(dto.WSOutboundMessage{
		Type: "pong",
		Time: time.Now().Format(time.RFC3339),
	})
	//发给前端
	c.Send <- pongMsg
}

// Go 结构体
// ↓
// json.Marshal → 变成 []byte（JSON 字节）
// ↓
// 放进 Send 通道
// ↓
// writePump 从通道取出来
// ↓
// 通过 WebSocket 发给前端
func (c *WSClient) sendWSError(message string) {
	errorMsg, _ := json.Marshal(dto.WSOutboundMessage{
		Type:    "error",
		Message: message,
		Time:    time.Now().Format(time.RFC3339),
	})
	c.Send <- errorMsg
}
