﻿package api

import (
	"encoding/json"
	"go-admin/core"
	"go-admin/internal/repository"
	"go-admin/model"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/gorilla/websocket"
)

type Client struct {
	Hub    *Hub
	Conn   *websocket.Conn
	UserID uint
	Send   chan []byte
}

type Hub struct {
	Clients     map[uint]*Client
	Register    chan *Client
	Unregister  chan *Client
	Mutex       sync.RWMutex
	MessageRepo repository.MessageRepository
}

func NewHub(messageRepo repository.MessageRepository) *Hub {
	return &Hub{
		Clients:     make(map[uint]*Client),
		Register:    make(chan *Client),
		Unregister:  make(chan *Client),
		MessageRepo: messageRepo,
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.Mutex.Lock()
			if old, ok := h.Clients[client.UserID]; ok {
				close(old.Send)
				old.Conn.Close()
			}
			h.Clients[client.UserID] = client
			h.Mutex.Unlock()
			log.Printf("user %d connected", client.UserID)
		case client := <-h.Unregister:
			h.Mutex.Lock()
			if c, ok := h.Clients[client.UserID]; ok && c == client {
				close(client.Send)
				delete(h.Clients, client.UserID)
			}
			h.Mutex.Unlock()
			log.Printf("user %d disconnected", client.UserID)
		}
	}
}

func (h *Hub) SendMessageToUser(userID uint, message []byte) {
	h.Mutex.RLock()
	client, ok := h.Clients[userID]
	h.Mutex.RUnlock()

	if ok {
		select {
		case client.Send <- message:
		default:
			close(client.Send)
			delete(h.Clients, userID)
		}
	}
}

type WSAPI struct {
	hub       *Hub
	jwtSecret []byte
	upgrader  websocket.Upgrader
}

func NewWSAPI(messageRepo repository.MessageRepository, jwtSecret []byte) *WSAPI {
	hub := NewHub(messageRepo)
	go hub.Run()

	return &WSAPI{
		hub:       hub,
		jwtSecret: jwtSecret,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (api *WSAPI) HandleWebSocket(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		core.Fail(c, http.StatusUnauthorized, "missing token")
		return
	}

	claims := &jwt.RegisteredClaims{}
	parsedToken, err := jwt.ParseWithClaims(token, claims, func(token *jwt.Token) (interface{}, error) {
		return api.jwtSecret, nil
	})
	if err != nil || !parsedToken.Valid {
		core.Fail(c, http.StatusUnauthorized, "invalid token")
		return
	}

	conn, err := api.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}

	userIDStr := claims.Subject
	userIDUint, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil || userIDUint == 0 {
		conn.Close()
		return
	}

	client := &Client{
		Hub:    api.hub,
		Conn:   conn,
		UserID: uint(userIDUint),
		Send:   make(chan []byte, 256),
	}

	api.hub.Register <- client
	go client.writePump()
	go client.readPump()
}

func (c *Client) writePump() {
	defer func() {
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		}
	}
}

func (c *Client) readPump() {
	defer func() {
		c.Hub.Unregister <- c
		c.Conn.Close()
	}()

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			break
		}

		var msgData struct {
			Type    string `json:"type"`
			ToUID   uint   `json:"to_uid"`
			Content string `json:"content"`
		}

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

func (c *Client) handleMessage(toUID uint, content string) {
	_, err := c.Hub.MessageRepo.FindUserByID(toUID)
	if err != nil {
		errorMsg, _ := json.Marshal(gin.H{
			"type":    "error",
			"message": "user not found",
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

	receiverMsg, _ := json.Marshal(gin.H{
		"type":     "message",
		"from_uid": c.UserID,
		"content":  content,
		"time":     time.Now().Format(time.RFC3339),
	})
	c.Hub.SendMessageToUser(toUID, receiverMsg)

	confirmMsg, _ := json.Marshal(gin.H{
		"type":    "message_sent",
		"to_uid":  toUID,
		"content": content,
		"time":    time.Now().Format(time.RFC3339),
	})
	c.Send <- confirmMsg
}

func (c *Client) handlePing() {
	pongMsg, _ := json.Marshal(gin.H{
		"type": "pong",
		"time": time.Now().Format(time.RFC3339),
	})
	c.Send <- pongMsg
}

func (api *WSAPI) BroadcastMessage(message interface{}) {
	msgBytes, err := json.Marshal(message)
	if err != nil {
		return
	}

	api.hub.Mutex.RLock()
	defer api.hub.Mutex.RUnlock()

	for _, client := range api.hub.Clients {
		select {
		case client.Send <- msgBytes:
		default:
			close(client.Send)
			delete(api.hub.Clients, client.UserID)
		}
	}
}

func (api *WSAPI) GetOnlineUsers() []uint {
	api.hub.Mutex.RLock()
	defer api.hub.Mutex.RUnlock()

	onlineUsers := make([]uint, 0, len(api.hub.Clients))
	for userID := range api.hub.Clients {
		onlineUsers = append(onlineUsers, userID)
	}

	return onlineUsers
}
