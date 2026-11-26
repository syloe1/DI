package service

import (
	"context"
	"di/internal/domain/model"
	"di/internal/telemetry"
	"di/internal/transport"
	"net/http"
	"time"
)

// -------------------------- 群邀请：邀请入群 / 审核邀请 --------------------------

/*
邀请人必须是 active 成员
所有 active 成员都可邀请
invitee 必须存在
不能邀请自己
被邀请人如果已经 active，返回已在群中
重复 pending 邀请，返回已有邀请
邀请有效期 7 天
*/
func (s *GroupService) InviteMember(ctx context.Context, inviterUID, groupID, inviteeUID uint) (*model.ChatGroupInvitation, error) {
	if inviterUID == inviteeUID {
		return nil, transport.NewBizError(http.StatusBadRequest, "cannot invite yourself")
	}

	if _, err := s.getNormalGroup(ctx, groupID); err != nil {
		return nil, err
	}

	if _, err := s.getActiveMember(ctx, groupID, inviterUID, "not a group member"); err != nil {
		return nil, err
	}

	if _, err := s.groupRepo.FindUserByID(ctx, inviteeUID); err != nil {
		return nil, transport.NewBizError(http.StatusNotFound, "invitee user not found")
	}

	if member, err := s.groupRepo.FindMember(ctx, groupID, inviteeUID); err == nil && member.Status == model.ChatGroupMemberStatusActive {
		return nil, transport.NewBizError(http.StatusBadRequest, "invitee already in group")
	}

	if invitation, err := s.groupRepo.FindPendingInvitation(ctx, groupID, inviteeUID); err == nil {
		return invitation, nil
	}

	invitation := &model.ChatGroupInvitation{
		GroupID:    groupID,
		InviterUID: inviterUID,
		InviteeUID: inviteeUID,
		Status:     model.ChatGroupInvitationStatusPending,
		ExpiredAt:  time.Now().Add(7 * 24 * time.Hour),
	}

	if err := s.groupRepo.CreateInvitation(ctx, invitation); err != nil {
		return nil, transport.NewBizError(http.StatusInternalServerError, "create invitation failed")
	}

	return invitation, nil
}

/*
只有 invitee 本人能处理邀请
只能处理 pending
过期后不能接受，状态改 expired
accept：邀请状态 accepted，并加入/恢复成员
reject：邀请状态 rejected
handled_at = now
*/
func (s *GroupService) ReviewInvitation(ctx context.Context, inviteeUID, invitationID uint, action string) error {
	invitation, err := s.groupRepo.FindInvitationByID(ctx, invitationID)
	if err != nil {
		return transport.NewBizError(http.StatusNotFound, "invitation not found")
	}

	if invitation.InviteeUID != inviteeUID {
		return transport.NewBizError(http.StatusForbidden, "can only review your own invitation")
	}

	if invitation.Status != model.ChatGroupInvitationStatusPending {
		return transport.NewBizError(http.StatusBadRequest, "invitation is not pending")
	}

	if _, err := s.getNormalGroup(ctx, invitation.GroupID); err != nil {
		return err
	}

	now := time.Now()
	if now.After(invitation.ExpiredAt) {
		invitation.Status = model.ChatGroupInvitationStatusExpired
		invitation.HandledAt = &now
		if err := s.groupRepo.UpdateInvitation(ctx, invitation); err != nil {
			telemetry.Z().Errorf("mark invitation expired failed invitation_id=%d: %v", invitationID, err)
		}
		return transport.NewBizError(http.StatusBadRequest, "invitation expired")
	}

	switch action {
	case "accept":
		invitation.Status = model.ChatGroupInvitationStatusAccepted
		invitation.HandledAt = &now

		member := &model.ChatGroupMember{
			GroupID:  invitation.GroupID,
			UserID:   inviteeUID,
			Role:     model.ChatGroupMemberRoleMember,
			Status:   model.ChatGroupMemberStatusActive,
			JoinedAt: now,
			LeftAt:   nil,
		}

		if err := s.groupRepo.AcceptInvitation(ctx, invitation, member); err != nil {
			return transport.NewBizError(http.StatusInternalServerError, "accept invitation failed")
		}
		s.refreshMembershipCache(ctx, invitation.GroupID)

	case "reject":
		invitation.Status = model.ChatGroupInvitationStatusRejected
		invitation.HandledAt = &now

		if err := s.groupRepo.UpdateInvitation(ctx, invitation); err != nil {
			return transport.NewBizError(http.StatusInternalServerError, "reject invitation failed")
		}

	default:
		return transport.NewBizError(http.StatusBadRequest, "invalid action")
	}

	return nil
}
