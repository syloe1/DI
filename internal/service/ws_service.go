package service

import (
	"context"
	"encoding/json"
	"net/http"

	"go-admin/internal/dao"
	"go-admin/pkg/core"

	"github.com/golang-jwt/jwt/v4"
	"github.com/gorilla/websocket"
)

type WSService struct {
	hub       *WSHub
	jwtSecret []byte
}

func NewWSService(messageRepo dao.MessageRepository, cache dao.UserCache, ctx context.Context, jwtSecret []byte) *WSService {
	hub := NewWSHub(messageRepo, cache, ctx)
	go hub.Run()

	return &WSService{
		hub:       hub,
		jwtSecret: jwtSecret,
	}
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
	msgBytes, err := json.Marshal(message)
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
			close(client.Send)
			s.hub.Mutex.Lock()
			delete(s.hub.Clients, client.UserID)
			s.hub.Mutex.Unlock()
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
