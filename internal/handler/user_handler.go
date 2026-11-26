package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"di/internal/dto"
	"di/internal/service"
	"di/internal/telemetry"
	"di/internal/transport"
	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	svc *service.UserService
}

func NewUserHandler(svc *service.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

func (h *UserHandler) Register(c *gin.Context) {
	var req dto.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.FailByError(c, transport.ParseBindError(err))
		return
	}

	err := h.svc.Register(c.Request.Context(), req.Username, req.Password, req.InviteCode)
	if err != nil {
		if errors.Is(err, service.ErrUserExists) {
			transport.Fail(c, http.StatusBadRequest, "用户名已存在")
			return
		}
		// 审计：失败注册（邀请码错误等）可追溯，避免批量注册无日志
		telemetry.Z().Warnw("user register rejected",
			"username", req.Username, "client_ip", c.ClientIP(), "reason", err.Error())
		transport.FailByError(c, err)
		return
	}

	telemetry.Z().Infow("user registered", "username", req.Username, "client_ip", c.ClientIP())
	transport.SuccessWithMessage(c, "register success", nil)
}

// RegisterConfig 公开配置：前端据此决定是否展示邀请码输入框 / 体验 Demo 入口。
func (h *UserHandler) RegisterConfig(c *gin.Context) {
	transport.Success(c, gin.H{
		"invite_required": h.svc.RegisterInviteRequired(),
		"demo_enabled":    h.svc.DemoLoginEnabled(),
	})
}

// DemoLogin 免密登录预置演示账号（demo.enabled=true 时可用）。
func (h *UserHandler) DemoLogin(c *gin.Context) {
	token, role, err := h.svc.DemoLogin(c.Request.Context())
	if err != nil {
		transport.FailByError(c, err)
		return
	}
	telemetry.Z().Infow("demo login", "client_ip", c.ClientIP())
	transport.Success(c, gin.H{
		"token": token,
		"role":  role,
		"demo":  true,
	})
}

func (h *UserHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.FailByError(c, transport.ParseBindError(err))
		return
	}

	token, role, err := h.svc.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	transport.Success(c, gin.H{
		"token": token,
		"role":  role,
	})
}

func (h *UserHandler) Logout(c *gin.Context) {
	userID := c.GetUint("userID")
	token := c.GetString("token")
	expiresAt, _ := c.Get("tokenExpiresAt")
	var exp time.Time
	if t, ok := expiresAt.(time.Time); ok {
		exp = t
	}
	if err := h.svc.Logout(c.Request.Context(), userID, token, exp); err != nil {
		transport.FailByError(c, err)
		return
	}
	transport.SuccessWithMessage(c, "logout success", nil)
}

func (h *UserHandler) AddUser(c *gin.Context) {
	var req dto.AddUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.FailByError(c, transport.ParseBindError(err))
		return
	}

	err := h.svc.AddUser(c.Request.Context(), c.GetString("role"), req.Username, req.Password, req.Role)
	if err != nil {
		if errors.Is(err, service.ErrUserExists) {
			transport.Fail(c, http.StatusBadRequest, "username already exists")
			return
		}
		transport.FailByError(c, err)
		return
	}

	transport.SuccessWithMessage(c, "add user success", nil)
}

func (h *UserHandler) GetUserList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	data, err := h.svc.GetUserList(c.Request.Context(), c.GetString("role"), page, pageSize)
	if err != nil {
		transport.FailByError(c, err)
		return
	}
	transport.Success(c, data)
}

func (h *UserHandler) GetUser(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	user, err := h.svc.GetUser(c.Request.Context(), id)
	if err != nil {
		transport.FailByError(c, err)
		return
	}
	transport.Success(c, user)
}

func (h *UserHandler) UpdateUser(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	var req dto.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.FailByError(c, transport.ParseBindError(err))
		return
	}

	if err := h.svc.UpdateUser(c.Request.Context(), id, c.GetUint("userID"), c.GetString("role"), req.Username, req.Role); err != nil {
		transport.FailByError(c, err)
		return
	}
	transport.SuccessWithMessage(c, "update success", nil)
}

func (h *UserHandler) ChangePassword(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	var req dto.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.FailByError(c, transport.ParseBindError(err))
		return
	}

	if err := h.svc.ChangePassword(c.Request.Context(), id, c.GetUint("userID"), c.GetString("role"), req.OldPassword, req.NewPassword); err != nil {
		transport.FailByError(c, err)
		return
	}
	transport.SuccessWithMessage(c, "password updated", nil)
}

func (h *UserHandler) DeleteUser(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	if err := h.svc.DeleteUser(c.Request.Context(), c.GetString("role"), id); err != nil {
		transport.FailByError(c, err)
		return
	}
	transport.SuccessWithMessage(c, "delete success", nil)
}

func (h *UserHandler) SearchUser(c *gin.Context) {
	data, err := h.svc.SearchUser(c.Request.Context(), c.Query("username"))
	if err != nil {
		transport.FailByError(c, err)
		return
	}
	transport.Success(c, data)
}

func (h *UserHandler) GetUserOnlineStatus(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	data, err := h.svc.GetUserOnlineStatus(c.Request.Context(), id)
	if err != nil {
		transport.FailByError(c, err)
		return
	}
	transport.Success(c, data)
}

func (h *UserHandler) GetOnlineUsers(c *gin.Context) {
	data, err := h.svc.GetOnlineUsers(c.Request.Context())
	if err != nil {
		transport.FailByError(c, err)
		return
	}
	transport.Success(c, data)
}

func (h *UserHandler) BatchGetUserRoles(c *gin.Context) {
	var req dto.BatchGetUserRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.FailByError(c, transport.ParseBindError(err))
		return
	}

	data, err := h.svc.BatchGetUserRoles(c.Request.Context(), req.IDs)
	if err != nil {
		transport.FailByError(c, err)
		return
	}
	transport.Success(c, data)
}
func (h *UserHandler) BatchPutUserRoles(c *gin.Context) {
	var req dto.BatchPutUserRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.FailByError(c, transport.ParseBindError(err))
		return
	}

	currentUserID := c.GetUint("userID")
	currentRole := c.GetString("role")

	err := h.svc.BatchPutUserRoles(c.Request.Context(), currentUserID, currentRole, req.IDs, req.Role)
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	transport.Success(c, nil)
}

func parseUintParam(c *gin.Context, key string) (uint, bool) {
	id64, err := strconv.ParseUint(c.Param(key), 10, 64)
	if err != nil || id64 == 0 {
		transport.Fail(c, http.StatusBadRequest, "invalid user id")
		return 0, false
	}
	return uint(id64), true
}
