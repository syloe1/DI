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

func NewWSHandler(svc *service.WSService) *WSHandler {
	return &WSHandler{
		svc: svc,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
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
