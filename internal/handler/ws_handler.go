package handler

import (
	"log"
	"net/http"
	"strings"

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

// extractWSToken 从 WebSocket 握手请求中提取 JWT（P0-3 修复）：
//  1. 优先 Sec-WebSocket-Protocol 子协议（推荐：不污染 URL，不进访问日志/浏览器历史）
//  2. 兼容 Authorization: Bearer 头
//  3. 兜底 URL query ?token=（仅兼容旧客户端，不建议）
func extractWSToken(r *http.Request) string {
	if proto := r.Header.Get("Sec-WebSocket-Protocol"); proto != "" {
		// 浏览器可能发送多个子协议（逗号分隔），约定 token 放首位
		if i := strings.IndexByte(proto, ','); i >= 0 {
			return strings.TrimSpace(proto[:i])
		}
		return strings.TrimSpace(proto)
	}
	if auth := r.Header.Get("Authorization"); auth != "" {
		return strings.TrimPrefix(strings.TrimSpace(auth), "Bearer ")
	}
	return r.URL.Query().Get("token")
}

func (h *WSHandler) HandleWebSocket(c *gin.Context) {
	token := extractWSToken(c.Request)
	userID, err := h.svc.AuthenticateToken(token)
	if err != nil {
		core.FailByError(c, err)
		return
	}

	// 若客户端通过子协议传 token，握手响应必须回显同一子协议，否则浏览器拒绝连接
	respHeader := http.Header{}
	if proto := c.Request.Header.Get("Sec-WebSocket-Protocol"); proto != "" {
		respHeader.Set("Sec-WebSocket-Protocol", token)
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, respHeader)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}

	h.svc.RegisterConnection(conn, userID)
}
