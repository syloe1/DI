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
	default:
		h.dropSlowClient(client)
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

func (h *WSHub) registerClient(client *WSClient) {
	h.Mutex.Lock()
	if old, ok := h.Clients[client.UserID]; ok && old != client {
		close(old.Send)
		_ = old.Conn.Close()
	}
	h.Clients[client.UserID] = client
	h.Mutex.Unlock()

	if h.Cache != nil {
		_ = h.Cache.SAdd(h.Ctx, OnlineUsersKey, client.UserID)
	}
	if h.Presence != nil {
		_ = h.Presence.RegisterOnline(h.Ctx, client.UserID)
	}
	if h.GroupCache != nil {
		_ = h.GroupCache.AddUserOnlineGroups(h.Ctx, client.UserID)
	}
	log.Printf("user %d connected", client.UserID)
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

	if h.Cache != nil {
		_ = h.Cache.SRem(h.Ctx, OnlineUsersKey, client.UserID)
	}
	if h.Presence != nil {
		_ = h.Presence.UnregisterOnline(h.Ctx, client.UserID)
	}
	if h.GroupCache != nil {
		_ = h.GroupCache.RemoveUserOnlineGroups(h.Ctx, client.UserID)
	}
	log.Printf("user %d disconnected", client.UserID)
}

// 但websocket客户端发送消息太慢， 主动断开
func (h *WSHub) dropSlowClient(client *WSClient) {
	log.Printf("drop slow websocket client user_id=%d", client.UserID)
	h.unregisterClient(client)
}

func (c *WSClient) writePump() {
	defer func() {
		c.Hub.Unregister <- c
		_ = c.Conn.Close()
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
		c.Hub.Unregister <- c
		_ = c.Conn.Close()
	}()

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			return
		}

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
	c.Hub.SendMessageToUser(toUID, receiverMsg)

	confirmMsg, _ := json.Marshal(dto.WSOutboundMessage{
		Type:    "message_sent",
		ToUID:   toUID,
		Content: content,
		Time:    now,
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
	if err := c.Hub.GroupRepo.CreateGroupMessage(message); err != nil {
		c.sendWSError("send group message failed")
		return
	}

	if c.Hub.GroupCache != nil {
		_ = c.Hub.GroupCache.RefreshActiveMembers(c.Hub.Ctx, groupID)
	}

	if c.Hub.Publisher == nil {
		log.Printf("group message publisher is nil")
		return
	}

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
	if c.Hub.Presence != nil {
		_ = c.Hub.Presence.RefreshOnline(c.Hub.Ctx, c.UserID)
	}
	pongMsg, _ := json.Marshal(dto.WSOutboundMessage{
		Type: "pong",
		Time: time.Now().Format(time.RFC3339),
	})
	c.Hub.SendMessageToUser(c.UserID, pongMsg)
}

func (c *WSClient) sendWSError(message string) {
	errorMsg, _ := json.Marshal(dto.WSOutboundMessage{
		Type:    "error",
		Message: message,
		Time:    time.Now().Format(time.RFC3339),
	})
	c.Hub.SendMessageToUser(c.UserID, errorMsg)
}
