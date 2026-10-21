package service

import (
	"go-admin/internal/domain/model"
	"go-admin/pkg/core"
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
func (s *GroupService) InviteMember(inviterUID, groupID, inviteeUID uint) (*model.ChatGroupInvitation, error) {
	if inviterUID == inviteeUID {
		return nil, core.NewBizError(http.StatusBadRequest, "cannot invite yourself")
	}

	if _, err := s.getNormalGroup(groupID); err != nil {
		return nil, err
	}

	if _, err := s.getActiveMember(groupID, inviterUID, "not a group member"); err != nil {
		return nil, err
	}

	if _, err := s.groupRepo.FindUserByID(inviteeUID); err != nil {
		return nil, core.NewBizError(http.StatusNotFound, "invitee user not found")
	}

	if member, err := s.groupRepo.FindMember(groupID, inviteeUID); err == nil && member.Status == model.ChatGroupMemberStatusActive {
		return nil, core.NewBizError(http.StatusBadRequest, "invitee already in group")
	}

	if invitation, err := s.groupRepo.FindPendingInvitation(groupID, inviteeUID); err == nil {
		return invitation, nil
	}

	invitation := &model.ChatGroupInvitation{
		GroupID:    groupID,
		InviterUID: inviterUID,
		InviteeUID: inviteeUID,
		Status:     model.ChatGroupInvitationStatusPending,
		ExpiredAt:  time.Now().Add(7 * 24 * time.Hour),
	}

	if err := s.groupRepo.CreateInvitation(invitation); err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "create invitation failed")
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
func (s *GroupService) ReviewInvitation(inviteeUID, invitationID uint, action string) error {
	invitation, err := s.groupRepo.FindInvitationByID(invitationID)
	if err != nil {
		return core.NewBizError(http.StatusNotFound, "invitation not found")
	}

	if invitation.InviteeUID != inviteeUID {
		return core.NewBizError(http.StatusForbidden, "can only review your own invitation")
	}

	if invitation.Status != model.ChatGroupInvitationStatusPending {
		return core.NewBizError(http.StatusBadRequest, "invitation is not pending")
	}

	if _, err := s.getNormalGroup(invitation.GroupID); err != nil {
		return err
	}

	now := time.Now()
	if now.After(invitation.ExpiredAt) {
		invitation.Status = model.ChatGroupInvitationStatusExpired
		invitation.HandledAt = &now
		_ = s.groupRepo.UpdateInvitation(invitation)
		return core.NewBizError(http.StatusBadRequest, "invitation expired")
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

		if err := s.groupRepo.AcceptInvitation(invitation, member); err != nil {
			return core.NewBizError(http.StatusInternalServerError, "accept invitation failed")
		}
		s.refreshMembershipCache(invitation.GroupID)

	case "reject":
		invitation.Status = model.ChatGroupInvitationStatusRejected
		invitation.HandledAt = &now

		if err := s.groupRepo.UpdateInvitation(invitation); err != nil {
			return core.NewBizError(http.StatusInternalServerError, "reject invitation failed")
		}

	default:
		return core.NewBizError(http.StatusBadRequest, "invalid action")
	}

	return nil
}
