package service

import (
	"context"
	"encoding/json"
	"log"
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
	Cache       dao.UserCache
	Ctx         context.Context
}

func NewWSHub(messageRepo dao.MessageRepository, cache dao.UserCache, ctx context.Context) *WSHub {
	return &WSHub{
		Clients:     make(map[uint]*WSClient),
		Register:    make(chan *WSClient),
		Unregister:  make(chan *WSClient),
		MessageRepo: messageRepo,
		Cache:       cache,
		Ctx:         ctx,
	}
}

func (h *WSHub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.Mutex.Lock()
			if old, ok := h.Clients[client.UserID]; ok {
				close(old.Send)
				_ = old.Conn.Close()
			}
			h.Clients[client.UserID] = client
			h.Mutex.Unlock()
			if h.Cache != nil {
				_ = h.Cache.SAdd(h.Ctx, OnlineUsersKey, client.UserID)
			}
			log.Printf("user %d connected", client.UserID)
		case client := <-h.Unregister:
			h.Mutex.Lock()
			if c, ok := h.Clients[client.UserID]; ok && c == client {
				close(client.Send)
				delete(h.Clients, client.UserID)
			}
			h.Mutex.Unlock()
			if h.Cache != nil {
				_ = h.Cache.SRem(h.Ctx, OnlineUsersKey, client.UserID)
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

func (c *WSClient) writePump() {
	defer func() {
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
			break
		}

		var msgData dto.WSInboundMessage
		if err := json.Unmarshal(message, &msgData); err != nil {
			continue
		}

		switch msgData.Type {
		case "message":
			c.handleMessage(msgData.ToUID, msgData.Content)
		case "ping":
			c.handlePing()
		}
	}
}

func (c *WSClient) handleMessage(toUID uint, content string) {
	_, err := c.Hub.MessageRepo.FindUserByID(toUID)
	if err != nil {
		errorMsg, _ := json.Marshal(dto.WSOutboundMessage{
			Type:    "error",
			Message: "user not found",
		})
		c.Send <- errorMsg
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
	c.Send <- confirmMsg
}

func (c *WSClient) handlePing() {
	pongMsg, _ := json.Marshal(dto.WSOutboundMessage{
		Type: "pong",
		Time: time.Now().Format(time.RFC3339),
	})
	c.Send <- pongMsg
}
