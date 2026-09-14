package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"go-admin/internal/dao"
	"go-admin/pkg/core"

	"github.com/golang-jwt/jwt/v4"
	"github.com/gorilla/websocket"
)

var bufPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

func marshalWithPool(v interface{}) ([]byte, error) {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)
	enc := json.NewEncoder(buf)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type WSService struct {
	hub       *WSHub
	jwtSecret []byte
}

func NewWSService(messageRepo dao.MessageRepository, groupRepo dao.GroupRepository, publisher GroupMessagePublisher, presence *PresenceService, groupCache *GroupCacheService, cache dao.UserCache, ctx context.Context, jwtSecret []byte) *WSService {
	hub := NewWSHub(messageRepo, groupRepo, publisher, presence, groupCache, cache, ctx)
	go hub.Run()

	return &WSService{
		hub:       hub,
		jwtSecret: jwtSecret,
	}
}

func (s *WSService) Hub() *WSHub {
	return s.hub
}

func (s *WSService) Shutdown(ctx context.Context) {
	s.hub.Shutdown(ctx)
}

func (s *WSService) AuthenticateToken(token string) (uint, error) {
	if token == "" {
		return 0, core.NewBizError(http.StatusUnauthorized, "missing token")
	}

	claims := jwt.MapClaims{}
	parsedToken, err := jwt.ParseWithClaims(token, claims, func(token *jwt.Token) (interface{}, error) {
		return s.jwtSecret, nil
	})
	if err != nil || !parsedToken.Valid {
		return 0, core.NewBizError(http.StatusUnauthorized, "invalid token")
	}

	userIDFloat, ok := claims["user_id"].(float64)
	if !ok || userIDFloat <= 0 {
		return 0, core.NewBizError(http.StatusUnauthorized, "invalid token payload")
	}

	return uint(userIDFloat), nil
}

func (s *WSService) RegisterConnection(conn *websocket.Conn, userID uint) {
	const pingTimeout = 60 * time.Second
	conn.SetReadDeadline(time.Now().Add(pingTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pingTimeout))
		return nil
	})
	client := &WSClient{
		Hub:    s.hub,
		Conn:   conn,
		UserID: userID,
		Send:   make(chan []byte, 256),
	}

	s.hub.Register <- client
	go client.writePump()
	go client.readPump()
}

func (s *WSService) BroadcastMessage(message interface{}) {
	msgBytes, err := marshalWithPool(message)
	if err != nil {
		return
	}

	s.hub.Mutex.RLock()
	clients := make([]*WSClient, 0, len(s.hub.Clients))
	for _, client := range s.hub.Clients {
		clients = append(clients, client)
	}
	s.hub.Mutex.RUnlock()

	for _, client := range clients {
		select {
		case client.Send <- msgBytes:
		default:
			// 慢客户端：不直接 close channel（二次 close 会 panic），
			// 交给 Hub 事件循环安全清理。
			select {
			case s.hub.Unregister <- client:
			default:
				go func(c *WSClient) { s.hub.Unregister <- c }(client)
			}
		}
	}
}

func (s *WSService) GetOnlineUsers() []uint {
	s.hub.Mutex.RLock()
	defer s.hub.Mutex.RUnlock()

	onlineUsers := make([]uint, 0, len(s.hub.Clients))
	for userID := range s.hub.Clients {
		onlineUsers = append(onlineUsers, userID)
	}
	return onlineUsers
}
