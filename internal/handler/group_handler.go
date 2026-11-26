package handler

import (
	"di/internal/dto"
	"di/internal/middleware"
	"di/internal/service"
	"di/internal/transport"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type GroupHandler struct {
	svc *service.GroupService
}

func NewGroupHandler(svc *service.GroupService) *GroupHandler {
	return &GroupHandler{svc: svc}
}

func (h *GroupHandler) CreateGroup(c *gin.Context) {
	var req dto.CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.FailByError(c, transport.ParseBindError(err))
		return
	}

	group, err := h.svc.CreateGroup(c.Request.Context(), c.GetUint("userID"), req)
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	transport.SuccessWithMessage(c, "create group success", group)
}

// SetAdmin 设置管理员
func (h *GroupHandler) SetAdmin(c *gin.Context) {
	// 解析群组 ID
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	// 绑定 JSON
	var req dto.SetGroupAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.FailByError(c, transport.ParseBindError(err))
		return
	}

	// 调用服务
	operatorUID := c.GetUint("userID")
	if err := h.svc.SetAdmin(c.Request.Context(), operatorUID, groupID, req.UserID); err != nil {
		transport.FailByError(c, err)
		return
	}

	transport.SuccessWithMessage(c, "set admin success", nil)
}

// CancelAdmin 取消管理员
func (h *GroupHandler) CancelAdmin(c *gin.Context) {
	// 解析参数（全部复用）
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	userID, err := h.parseUserIDFromParam(c)
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	// 调用服务
	operatorUID := c.GetUint("userID")
	if err := h.svc.CancelAdmin(c.Request.Context(), operatorUID, groupID, userID); err != nil {
		transport.FailByError(c, err)
		return
	}

	transport.SuccessWithMessage(c, "cancel admin success", nil)
}

// parseGroupIDFromParam 从 URL 参数 id 解析群组 ID（通用）
func (h *GroupHandler) parseGroupIDFromParam(c *gin.Context) (uint, error) {
	groupIDStr := c.Param("id")
	gid, err := strconv.ParseUint(groupIDStr, 10, 32)
	if err != nil {
		return 0, transport.NewBizError(http.StatusBadRequest, "invalid group id")
	}
	return uint(gid), nil
}

// parseUserIDFromParam 从 URL 参数 user_id 解析用户 ID（通用）
func (h *GroupHandler) parseUserIDFromParam(c *gin.Context) (uint, error) {
	userIDStr := c.Param("user_id")
	uid, err := strconv.ParseUint(userIDStr, 10, 32)
	if err != nil {
		return 0, transport.NewBizError(http.StatusBadRequest, "invalid user id")
	}
	return uint(uid), nil
}
func (h *GroupHandler) KickMember(c *gin.Context) {
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	userID, err := h.parseUserIDFromParam(c)
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	if err := h.svc.KickMember(c.Request.Context(), c.GetUint("userID"), groupID, userID); err != nil {
		transport.FailByError(c, err)
		return
	}

	transport.SuccessWithMessage(c, "kick member success", nil)
}

func (h *GroupHandler) TransferOwner(c *gin.Context) {
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	var req dto.TransferGroupOwnerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.FailByError(c, transport.ParseBindError(err))
		return
	}

	if err := h.svc.TransferOwner(c.Request.Context(), c.GetUint("userID"), groupID, req.UserID); err != nil {
		transport.FailByError(c, err)
		return
	}

	transport.SuccessWithMessage(c, "transfer owner success", nil)
}
func (h *GroupHandler) DissolveGroup(c *gin.Context) {
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	if err := h.svc.DissolveGroup(c.Request.Context(), c.GetUint("userID"), groupID); err != nil {
		transport.FailByError(c, err)
		return
	}

	transport.SuccessWithMessage(c, "dissolve group success", nil)
}

func (h *GroupHandler) InviteMember(c *gin.Context) {
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	var req dto.InviteGroupMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.FailByError(c, transport.ParseBindError(err))
		return
	}

	data, err := h.svc.InviteMember(c.Request.Context(), c.GetUint("userID"), groupID, req.InviteeID)
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	transport.SuccessWithMessage(c, "invite member success", data)
}
func (h *GroupHandler) ReviewInvitation(c *gin.Context) {
	invitationID, err := h.parseIDFromParam(c, "id", "invalid invitation id")
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	var req dto.ReviewGroupInvitationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.FailByError(c, transport.ParseBindError(err))
		return
	}

	if err := h.svc.ReviewInvitation(c.Request.Context(), c.GetUint("userID"), invitationID, req.Action); err != nil {
		transport.FailByError(c, err)
		return
	}

	transport.SuccessWithMessage(c, "review invitation success", nil)
}

func (h *GroupHandler) ApplyJoinGroup(c *gin.Context) {
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	var req dto.ApplyJoinGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.FailByError(c, transport.ParseBindError(err))
		return
	}

	data, err := h.svc.ApplyJoinGroup(c.Request.Context(), c.GetUint("userID"), groupID, req.Reason)
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	transport.SuccessWithMessage(c, "apply join group success", data)
}
func (h *GroupHandler) ReviewJoinRequest(c *gin.Context) {
	requestID, err := h.parseIDFromParam(c, "id", "invalid join request id")
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	var req dto.ReviewJoinRequestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.FailByError(c, transport.ParseBindError(err))
		return
	}

	if err := h.svc.ReviewJoinRequest(c.Request.Context(), c.GetUint("userID"), requestID, req.Action); err != nil {
		transport.FailByError(c, err)
		return
	}

	transport.SuccessWithMessage(c, "review join request success", nil)
}

func (h *GroupHandler) LeaveGroup(c *gin.Context) {
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	if err := h.svc.LeaveGroup(c.Request.Context(), c.GetUint("userID"), groupID); err != nil {
		transport.FailByError(c, err)
		return
	}

	transport.SuccessWithMessage(c, "leave group success", nil)
}
func (h *GroupHandler) parseIDFromParam(c *gin.Context, name string, message string) (uint, error) {
	val, err := strconv.ParseUint(c.Param(name), 10, 32)
	if err != nil {
		return 0, transport.NewBizError(http.StatusBadRequest, message)
	}
	return uint(val), nil
}

func (h *GroupHandler) GetGroupDetail(c *gin.Context) {
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	data, err := h.svc.GetGroupDetail(c.Request.Context(), c.GetUint("userID"), groupID)
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	transport.Success(c, data)
}

func (h *GroupHandler) GetGroupMembers(c *gin.Context) {
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	data, err := h.svc.GetGroupMembers(c.Request.Context(), c.GetUint("userID"), groupID)
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	transport.Success(c, data)
}

func (h *GroupHandler) GetMyGroups(c *gin.Context) {
	data, err := h.svc.GetMyGroups(c.Request.Context(), c.GetUint("userID"))
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	transport.Success(c, data)
}

func (h *GroupHandler) GetJoinRequests(c *gin.Context) {
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	data, err := h.svc.GetJoinRequests(c.Request.Context(), c.GetUint("userID"), groupID, page, pageSize)
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	transport.Success(c, data)
}
func (h *GroupHandler) SendGroupMessage(c *gin.Context) {
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	var req dto.SendGroupMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		transport.FailByError(c, transport.ParseBindError(err))
		return
	}

	data, err := h.svc.SendGroupMessage(c.Request.Context(), c.GetUint("userID"), groupID, req.Content, middleware.GetRequestID(c))
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	transport.SuccessWithMessage(c, "send group message success", data)
}
func (h *GroupHandler) GetGroupMessages(c *gin.Context) {
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		transport.FailByError(c, err)
		return
	}
	var query dto.GroupMessageListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		transport.FailByError(c, transport.ParseBindError(err))
		return
	}

	data, err := h.svc.GetGroupMessages(c.Request.Context(), c.GetUint("userID"), groupID, query.Page, query.PageSize)
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	transport.Success(c, data)
}

// SyncGroupMessages 群消息断线补偿：GET /auth/groups/:id/messages/sync?after_id=&limit=
func (h *GroupHandler) SyncGroupMessages(c *gin.Context) {
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		transport.FailByError(c, err)
		return
	}
	var query dto.GroupMessageSyncQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		transport.FailByError(c, transport.ParseBindError(err))
		return
	}

	data, err := h.svc.SyncGroupMessages(c.Request.Context(), c.GetUint("userID"), groupID, query.AfterID, query.Limit)
	if err != nil {
		transport.FailByError(c, err)
		return
	}

	transport.Success(c, data)
}
