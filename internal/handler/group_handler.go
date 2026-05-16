package handler

import (
	"go-admin/internal/dto"
	"go-admin/internal/service"
	"go-admin/pkg/core"
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
		core.FailByError(c, core.ParseBindError(err))
		return
	}

	group, err := h.svc.CreateGroup(c.GetUint("userID"), req)
	if err != nil {
		core.FailByError(c, err)
		return
	}

	core.SuccessWithMessage(c, "create group success", group)
}

// SetAdmin 设置管理员
func (h *GroupHandler) SetAdmin(c *gin.Context) {
	// 解析群组 ID
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		core.FailByError(c, err)
		return
	}

	// 绑定 JSON
	var req dto.SetGroupAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		core.FailByError(c, core.ParseBindError(err))
		return
	}

	// 调用服务
	operatorUID := c.GetUint("userID")
	if err := h.svc.SetAdmin(operatorUID, groupID, req.UserID); err != nil {
		core.FailByError(c, err)
		return
	}

	core.SuccessWithMessage(c, "set admin success", nil)
}

// CancelAdmin 取消管理员
func (h *GroupHandler) CancelAdmin(c *gin.Context) {
	// 解析参数（全部复用）
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		core.FailByError(c, err)
		return
	}

	userID, err := h.parseUserIDFromParam(c)
	if err != nil {
		core.FailByError(c, err)
		return
	}

	// 调用服务
	operatorUID := c.GetUint("userID")
	if err := h.svc.CancelAdmin(operatorUID, groupID, userID); err != nil {
		core.FailByError(c, err)
		return
	}

	core.SuccessWithMessage(c, "cancel admin success", nil)
}

// parseGroupIDFromParam 从 URL 参数 id 解析群组 ID（通用）
func (h *GroupHandler) parseGroupIDFromParam(c *gin.Context) (uint, error) {
	groupIDStr := c.Param("id")
	gid, err := strconv.ParseUint(groupIDStr, 10, 32)
	if err != nil {
		return 0, core.NewBizError(http.StatusBadRequest, "invalid group id")
	}
	return uint(gid), nil
}

// parseUserIDFromParam 从 URL 参数 user_id 解析用户 ID（通用）
func (h *GroupHandler) parseUserIDFromParam(c *gin.Context) (uint, error) {
	userIDStr := c.Param("user_id")
	uid, err := strconv.ParseUint(userIDStr, 10, 32)
	if err != nil {
		return 0, core.NewBizError(http.StatusBadRequest, "invalid user id")
	}
	return uint(uid), nil
}
func (h *GroupHandler) KickMember(c *gin.Context) {
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		core.FailByError(c, err)
		return
	}

	userID, err := h.parseUserIDFromParam(c)
	if err != nil {
		core.FailByError(c, err)
		return
	}

	if err := h.svc.KickMember(c.GetUint("userID"), groupID, userID); err != nil {
		core.FailByError(c, err)
		return
	}

	core.SuccessWithMessage(c, "kick member success", nil)
}

func (h *GroupHandler) TransferOwner(c *gin.Context) {
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		core.FailByError(c, err)
		return
	}

	var req dto.TransferGroupOwnerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		core.FailByError(c, core.ParseBindError(err))
		return
	}

	if err := h.svc.TransferOwner(c.GetUint("userID"), groupID, req.UserID); err != nil {
		core.FailByError(c, err)
		return
	}

	core.SuccessWithMessage(c, "transfer owner success", nil)
}
func (h *GroupHandler) DissolveGroup(c *gin.Context) {
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		core.FailByError(c, err)
		return
	}

	if err := h.svc.DissolveGroup(c.GetUint("userID"), groupID); err != nil {
		core.FailByError(c, err)
		return
	}

	core.SuccessWithMessage(c, "dissolve group success", nil)
}

func (h *GroupHandler) InviteMember(c *gin.Context) {
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		core.FailByError(c, err)
		return
	}

	var req dto.InviteGroupMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		core.FailByError(c, core.ParseBindError(err))
		return
	}

	data, err := h.svc.InviteMember(c.GetUint("userID"), groupID, req.InviteeID)
	if err != nil {
		core.FailByError(c, err)
		return
	}

	core.SuccessWithMessage(c, "invite member success", data)
}
func (h *GroupHandler) ReviewInvitation(c *gin.Context) {
	invitationID, err := h.parseIDFromParam(c, "id", "invalid invitation id")
	if err != nil {
		core.FailByError(c, err)
		return
	}

	var req dto.ReviewGroupInvitationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		core.FailByError(c, core.ParseBindError(err))
		return
	}

	if err := h.svc.ReviewInvitation(c.GetUint("userID"), invitationID, req.Action); err != nil {
		core.FailByError(c, err)
		return
	}

	core.SuccessWithMessage(c, "review invitation success", nil)
}

func (h *GroupHandler) ApplyJoinGroup(c *gin.Context) {
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		core.FailByError(c, err)
		return
	}

	var req dto.ApplyJoinGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		core.FailByError(c, core.ParseBindError(err))
		return
	}

	data, err := h.svc.ApplyJoinGroup(c.GetUint("userID"), groupID, req.Reason)
	if err != nil {
		core.FailByError(c, err)
		return
	}

	core.SuccessWithMessage(c, "apply join group success", data)
}
func (h *GroupHandler) ReviewJoinRequest(c *gin.Context) {
	requestID, err := h.parseIDFromParam(c, "id", "invalid join request id")
	if err != nil {
		core.FailByError(c, err)
		return
	}

	var req dto.ReviewJoinRequestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		core.FailByError(c, core.ParseBindError(err))
		return
	}

	if err := h.svc.ReviewJoinRequest(c.GetUint("userID"), requestID, req.Action); err != nil {
		core.FailByError(c, err)
		return
	}

	core.SuccessWithMessage(c, "review join request success", nil)
}

func (h *GroupHandler) LeaveGroup(c *gin.Context) {
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		core.FailByError(c, err)
		return
	}

	if err := h.svc.LeaveGroup(c.GetUint("userID"), groupID); err != nil {
		core.FailByError(c, err)
		return
	}

	core.SuccessWithMessage(c, "leave group success", nil)
}
func (h *GroupHandler) parseIDFromParam(c *gin.Context, name string, message string) (uint, error) {
	val, err := strconv.ParseUint(c.Param(name), 10, 32)
	if err != nil {
		return 0, core.NewBizError(http.StatusBadRequest, message)
	}
	return uint(val), nil
}

func (h *GroupHandler) GetGroupDetail(c *gin.Context) {
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		core.FailByError(c, err)
		return
	}

	data, err := h.svc.GetGroupDetail(c.GetUint("userID"), groupID)
	if err != nil {
		core.FailByError(c, err)
		return
	}

	core.Success(c, data)
}

func (h *GroupHandler) GetGroupMembers(c *gin.Context) {
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		core.FailByError(c, err)
		return
	}

	data, err := h.svc.GetGroupMembers(c.GetUint("userID"), groupID)
	if err != nil {
		core.FailByError(c, err)
		return
	}

	core.Success(c, data)
}

func (h *GroupHandler) GetMyGroups(c *gin.Context) {
	data, err := h.svc.GetMyGroups(c.GetUint("userID"))
	if err != nil {
		core.FailByError(c, err)
		return
	}

	core.Success(c, data)
}

func (h *GroupHandler) GetJoinRequests(c *gin.Context) {
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		core.FailByError(c, err)
		return
	}

	data, err := h.svc.GetJoinRequests(c.GetUint("userID"), groupID)
	if err != nil {
		core.FailByError(c, err)
		return
	}

	core.Success(c, data)
}
func (h *GroupHandler) SendGroupMessage(c *gin.Context) {
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		core.FailByError(c, err)
		return
	}

	var req dto.SendGroupMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		core.FailByError(c, core.ParseBindError(err))
		return
	}

	data, err := h.svc.SendGroupMessage(c.GetUint("userID"), groupID, req.Content)
	if err != nil {
		core.FailByError(c, err)
		return
	}

	core.SuccessWithMessage(c, "send group message success", data)
}
func (h *GroupHandler) GetGroupMessages(c *gin.Context) {
	groupID, err := h.parseGroupIDFromParam(c)
	if err != nil {
		core.FailByError(c, err)
		return
	}
	var query dto.GroupMesssageListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		core.FailByError(c, core.ParseBindError(err))
		return
	}

	data, err := h.svc.GetGroupMessages(c.GetUint("userID"), groupID, query.Page, query.PageSize)
	if err != nil {
		core.FailByError(c, err)
		return
	}

	core.Success(c, data)
}
