package handler

import (
	"log"
	"net/http"

	"go-admin/internal/service"
	"go-admin/pkg/core"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type WSHandler struct {
	svc      *service.WSService
	upgrader websocket.Upgrader
}

// allowedWSOrigins 是 WebSocket 连接允许的 Origin 主机名白名单。
// 开发环境放行 localhost；生产环境应通过配置文件或环境变量注入。
var allowedWSOrigins = []string{
	"localhost",
	"127.0.0.1",
}

// originHost 从 Origin URL 中提取主机部分。
// 例如 "http://localhost:3000" → "localhost"。
func originHost(origin string) string {
	// 跳过协议前缀 "http://" 或 "https://"
	if len(origin) > 7 && origin[:7] == "http://" {
		origin = origin[7:]
	} else if len(origin) > 8 && origin[:8] == "https://" {
		origin = origin[8:]
	}
	// 截断端口和路径
	for i, c := range origin {
		if c == ':' || c == '/' {
			return origin[:i]
		}
	}
	return origin
}

func isWSOriginAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// 非浏览器客户端（curl / wscat / 原生 App）通常不发送 Origin，放行
		return true
	}

	// 同源请求：Origin == Host（含端口）
	if origin == "http://"+r.Host || origin == "https://"+r.Host {
		return true
	}

	// 主机名白名单匹配
	host := originHost(origin)
	for _, allowed := range allowedWSOrigins {
		if host == allowed {
			return true
		}
	}

	return false
}

func NewWSHandler(svc *service.WSService) *WSHandler {
	return &WSHandler{
		svc: svc,
		upgrader: websocket.Upgrader{
			CheckOrigin: isWSOriginAllowed,
		},
	}
}

func (h *WSHandler) HandleWebSocket(c *gin.Context) {
	userID, err := h.svc.AuthenticateToken(c.Query("token"))
	if err != nil {
		core.FailByError(c, err)
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}

	h.svc.RegisterConnection(conn, userID)
}
