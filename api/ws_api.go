package api

import (
	"encoding/json"
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

// Client 表示一个WebSocket客户端
type Client struct {
	Hub    *Hub
	Conn   *websocket.Conn
	UserID uint
	Send   chan []byte
}

// Hub 管理所有WebSocket连接
type Hub struct {
	Clients     map[uint]*Client
	Register    chan *Client
	Unregister  chan *Client
	Mutex       sync.RWMutex
	MessageRepo repository.MessageRepository // 注入的Repository接口
}

// NewHub 创建新的Hub实例，注入依赖
func NewHub(messageRepo repository.MessageRepository) *Hub {
	return &Hub{
		Clients:     make(map[uint]*Client),
		Register:    make(chan *Client),
		Unregister:  make(chan *Client),
		MessageRepo: messageRepo,
	}
}

// Run 启动Hub的消息处理循环
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

			log.Printf("用户 %d 已连接", client.UserID)

		case client := <-h.Unregister:
			h.Mutex.Lock()
			if c, ok := h.Clients[client.UserID]; ok && c == client {
				close(client.Send)
				delete(h.Clients, client.UserID)
			}
			h.Mutex.Unlock()

			log.Printf("用户 %d 已断开连接", client.UserID)
		}
	}
}

// SendMessageToUser 向指定用户发送消息
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

// WSAPI 封装WebSocket相关的处理器，持有依赖
type WSAPI struct {
	hub       *Hub
	jwtSecret []byte
	upgrader  websocket.Upgrader
}

// NewWSAPI 构造函数：注入依赖
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

// HandleWebSocket WebSocket连接处理
func (api *WSAPI) HandleWebSocket(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "缺少token"})
		return
	}

	// 解析JWT token
	claims := &jwt.RegisteredClaims{}
	parsedToken, err := jwt.ParseWithClaims(token, claims, func(token *jwt.Token) (interface{}, error) {
		return api.jwtSecret, nil
	})

	if err != nil || !parsedToken.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "token无效"})
		return
	}

	// 升级到WebSocket连接
	conn, err := api.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket升级失败: %v", err)
		return
	}

	// 从token中获取用户ID
	userIDStr := claims.Subject
	userIDUint, err := strconv.ParseUint(userIDStr, 10, 64)
	if err != nil || userIDUint == 0 {
		conn.Close()
		return
	}
	userID := uint(userIDUint)

	// 创建客户端
	client := &Client{
		Hub:    api.hub,
		Conn:   conn,
		UserID: userID,
		Send:   make(chan []byte, 256),
	}

	// 注册客户端
	api.hub.Register <- client

	// 启动读写goroutine
	go client.writePump()
	go client.readPump()
}

// writePump 处理消息发送
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

// readPump 处理消息接收
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

		// 解析消息
		var msgData struct {
			Type    string `json:"type"`
			ToUID   uint   `json:"to_uid"`
			Content string `json:"content"`
		}

		if err := json.Unmarshal(message, &msgData); err != nil {
			continue
		}

		// 处理不同类型的消息
		switch msgData.Type {
		case "message":
			c.handleMessage(msgData.ToUID, msgData.Content)
		case "ping":
			c.handlePing()
		}
	}
}

// handleMessage 处理聊天消息
func (c *Client) handleMessage(toUID uint, content string) {
	// 检查对方用户是否存在
	_, err := c.Hub.MessageRepo.FindUserByID(toUID)
	if err != nil {
		// 发送错误消息给发送者
		errorMsg, _ := json.Marshal(gin.H{
			"type":    "error",
			"message": "对方用户不存在",
		})
		c.Send <- errorMsg
		return
	}

	// 创建消息记录
	message := &model.Message{
		FromUID: c.UserID,
		ToUID:   toUID,
		Content: content,
		IsRead:  false,
	}

	// 保存到数据库
	if err := c.Hub.MessageRepo.CreateMessage(message); err != nil {
		log.Printf("保存消息失败: %v", err)
		return
	}

	// 构建发送给接收者的消息
	receiverMsg, _ := json.Marshal(gin.H{
		"type":     "message",
		"from_uid": c.UserID,
		"content":  content,
		"time":     time.Now().Format(time.RFC3339),
	})

	// 发送给接收者
	c.Hub.SendMessageToUser(toUID, receiverMsg)

	// 发送确认消息给发送者
	confirmMsg, _ := json.Marshal(gin.H{
		"type":    "message_sent",
		"to_uid":  toUID,
		"content": content,
		"time":    time.Now().Format(time.RFC3339),
	})
	c.Send <- confirmMsg
}

// handlePing 处理心跳包
func (c *Client) handlePing() {
	pongMsg, _ := json.Marshal(gin.H{
		"type": "pong",
		"time": time.Now().Format(time.RFC3339),
	})
	c.Send <- pongMsg
}

// BroadcastMessage 广播消息给所有在线用户
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

// GetOnlineUsers 获取在线用户列表
func (api *WSAPI) GetOnlineUsers() []uint {
	api.hub.Mutex.RLock()
	defer api.hub.Mutex.RUnlock()

	onlineUsers := make([]uint, 0, len(api.hub.Clients))
	for userID := range api.hub.Clients {
		onlineUsers = append(onlineUsers, userID)
	}

	return onlineUsers
}
