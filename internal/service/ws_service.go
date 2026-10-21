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

// wsTokenRevoker WebSocket 握手时的 token 主动失效检查（与 HTTP 中间件共用实现）。
type wsTokenRevoker interface {
	IsRevoked(ctx context.Context, token string, userID uint, issuedAt time.Time) (bool, error)
}

type WSService struct {
	hub       *WSHub
	jwtSecret []byte
	revoker   wsTokenRevoker
}

func NewWSService(messageRepo dao.MessageRepository, groupRepo dao.GroupRepository, publisher GroupMessagePublisher, presence *PresenceService, groupCache *GroupCacheService, cache dao.UserCache, ctx context.Context, jwtSecret []byte, revoker wsTokenRevoker) *WSService {
	hub := NewWSHub(messageRepo, groupRepo, publisher, presence, groupCache, cache, ctx)
	go hub.Run()

	return &WSService{
		hub:       hub,
		jwtSecret: jwtSecret,
		revoker:   revoker,
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
		// 与 HTTP 中间件保持一致：只接受 HMAC 签名，防算法混淆
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return s.jwtSecret, nil
	})
	if err != nil || !parsedToken.Valid {
		return 0, core.NewBizError(http.StatusUnauthorized, "invalid token")
	}

	userIDFloat, ok := claims["user_id"].(float64)
	if !ok || userIDFloat <= 0 {
		return 0, core.NewBizError(http.StatusUnauthorized, "invalid token payload")
	}
	userID := uint(userIDFloat)

	// 登出/改密后的旧 token 不允许再建立 WS 连接
	if s.revoker != nil {
		issuedAt := time.Time{}
		// 优先毫秒级 iat_ms（新 token），老 token 退回秒级 iat
		if iatMs, ok := claims["iat_ms"].(float64); ok && iatMs > 0 {
			issuedAt = time.UnixMilli(int64(iatMs))
		} else if iat, ok := claims["iat"].(float64); ok {
			issuedAt = time.Unix(int64(iat), 0)
		}
		revoked, checkErr := s.revoker.IsRevoked(context.Background(), token, userID, issuedAt)
		if checkErr != nil {
			core.Z().Warnf("check ws token revocation failed: %v", checkErr)
		} else if revoked {
			return 0, core.NewBizError(http.StatusUnauthorized, "token revoked")
		}
	}

	return userID, nil
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

// SendToUser 向指定用户的所有在线连接推送事件（如已读回执）。
// 用户离线时静默返回（消息已落库，离线补偿由 sync 兜底）。
func (s *WSService) SendToUser(userID uint, message interface{}) {
	msgBytes, err := marshalWithPool(message)
	if err != nil {
		return
	}

	s.hub.Mutex.RLock()
	client, ok := s.hub.Clients[userID]
	s.hub.Mutex.RUnlock()
	if !ok {
		return
	}

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

// KickUser 强制断开用户连接（踢下线）。
// 关闭底层连接后，readPump 会退出并走 Hub 的 unregister 流程，
// 进而触发 presence 下线 + 群在线分组清理，保证在线状态一致。
func (s *WSService) KickUser(userID uint) {
	s.hub.Mutex.RLock()
	client, ok := s.hub.Clients[userID]
	s.hub.Mutex.RUnlock()
	if !ok {
		return
	}
	_ = client.Conn.Close()
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
