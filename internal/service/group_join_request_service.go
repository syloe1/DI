package service

import (
	"context"
	"di/internal/domain/model"
	"di/internal/telemetry"
	"di/internal/transport"
	"net/http"
	"strings"
	"time"
)

// -------------------------- 入群申请：申请 / 审批 / 申请列表 --------------------------

/*
用户通过 group_id 申请
群 normal
申请理由可选
已在群中不能申请
pending 重复申请返回已有申请
拒绝后 24 小时内不能再次申请
申请有效期 7 天
*/
func (s *GroupService) ApplyJoinGroup(ctx context.Context, userID, groupID uint, reason string) (*model.ChatGroupJoinRequest, error) {
	if _, err := s.getNormalGroup(ctx, groupID); err != nil {
		return nil, err
	}

	if _, err := s.groupRepo.FindUserByID(ctx, userID); err != nil {
		return nil, transport.NewBizError(http.StatusNotFound, "user not found")
	}

	if member, err := s.groupRepo.FindMember(ctx, groupID, userID); err == nil && member.Status == model.ChatGroupMemberStatusActive {
		return nil, transport.NewBizError(http.StatusBadRequest, "already in group")
	}

	if req, err := s.groupRepo.FindPendingJoinRequest(ctx, groupID, userID); err == nil {
		return req, nil
	}

	if rejected, err := s.groupRepo.FindLatestRejectedJoinRequest(ctx, groupID, userID); err == nil {
		if time.Since(rejected.CreatedAt) < 24*time.Hour {
			return nil, transport.NewBizError(http.StatusBadRequest, "can apply again after 24 hours")
		}
	}

	reason = strings.TrimSpace(reason)
	if len(reason) > 255 {
		return nil, transport.NewBizError(http.StatusBadRequest, "reason is too long")
	}

	req := &model.ChatGroupJoinRequest{
		GroupID:   groupID,
		UserID:    userID,
		Reason:    reason,
		Status:    model.ChatGroupJoinRequestStatusPending,
		ExpiredAt: time.Now().Add(7 * 24 * time.Hour),
	}

	if err := s.groupRepo.CreateJoinRequest(ctx, req); err != nil {
		return nil, transport.NewBizError(http.StatusInternalServerError, "create join request failed")
	}

	return req, nil
}

/*
只有 owner/admin 可以审批
申请必须 pending
申请未过期
approve：状态 approved，加入/恢复成员
reject：状态 rejected
reviewer_uid/reviewed_at 要写
*/
func (s *GroupService) ReviewJoinRequest(ctx context.Context, operatorUID, requestID uint, action string) error {
	req, err := s.groupRepo.FindJoinRequestByID(ctx, requestID)
	if err != nil {
		return transport.NewBizError(http.StatusNotFound, "join request not found")
	}

	if req.Status != model.ChatGroupJoinRequestStatusPending {
		return transport.NewBizError(http.StatusBadRequest, "join request is not pending")
	}

	if _, err := s.getNormalGroup(ctx, req.GroupID); err != nil {
		return err
	}

	operator, err := s.getActiveMember(ctx, req.GroupID, operatorUID, "not a group member")
	if err != nil {
		return err
	}

	if operator.Role != model.ChatGroupMemberRoleOwner && operator.Role != model.ChatGroupMemberRoleAdmin {
		return transport.NewBizError(http.StatusForbidden, "only owner or admin can review join request")
	}

	now := time.Now()
	if now.After(req.ExpiredAt) {
		req.Status = model.ChatGroupJoinRequestStatusExpired
		req.ReviewerUID = &operatorUID
		req.ReviewedAt = &now
		if err := s.groupRepo.UpdateJoinRequest(ctx, req); err != nil {
			telemetry.Z().Errorf("mark join request expired failed request_id=%d: %v", requestID, err)
		}
		return transport.NewBizError(http.StatusBadRequest, "join request expired")
	}

	req.ReviewerUID = &operatorUID
	req.ReviewedAt = &now

	switch action {
	case "approve":
		req.Status = model.ChatGroupJoinRequestStatusApproved

		member := &model.ChatGroupMember{
			GroupID:  req.GroupID,
			UserID:   req.UserID,
			Role:     model.ChatGroupMemberRoleMember,
			Status:   model.ChatGroupMemberStatusActive,
			JoinedAt: now,
			LeftAt:   nil,
		}

		if err := s.groupRepo.ApproveJoinRequest(ctx, req, member); err != nil {
			return transport.NewBizError(http.StatusInternalServerError, "approve join request failed")
		}
		s.refreshMembershipCache(ctx, req.GroupID)

	case "reject":
		req.Status = model.ChatGroupJoinRequestStatusRejected

		if err := s.groupRepo.UpdateJoinRequest(ctx, req); err != nil {
			return transport.NewBizError(http.StatusInternalServerError, "reject join request failed")
		}

	default:
		return transport.NewBizError(http.StatusBadRequest, "invalid action")
	}

	return nil
}

func (s *GroupService) GetJoinRequests(ctx context.Context, userID, groupID uint, page, pageSize int) ([]JoinRequestItem, error) {
	if _, err := s.getNormalGroup(ctx, groupID); err != nil {
		return nil, err
	}

	operator, err := s.getActiveMember(ctx, groupID, userID, "not a group member")
	if err != nil {
		return nil, err
	}

	if operator.Role != model.ChatGroupMemberRoleOwner && operator.Role != model.ChatGroupMemberRoleAdmin {
		return nil, transport.NewBizError(http.StatusForbidden, "only owner or admin can view join requests")
	}

	page, pageSize = normalizePagination(page, pageSize, 1, 20)
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize

	requests, err := s.groupRepo.ListJoinRequestsByGroupID(ctx, groupID, pageSize, offset)
	if err != nil {
		return nil, transport.NewBizError(http.StatusInternalServerError, "get join requests failed")
	}

	result := make([]JoinRequestItem, 0, len(requests))
	for _, req := range requests {
		result = append(result, JoinRequestItem{
			ID:          req.ID,
			GroupID:     req.GroupID,
			UserID:      req.UserID,
			Reason:      req.Reason,
			Status:      req.Status,
			ReviewerUID: req.ReviewerUID,
			ReviewedAt:  req.ReviewedAt,
			ExpiredAt:   req.ExpiredAt,
			CreatedAt:   req.CreatedAt,
		})
	}

	return result, nil
}
