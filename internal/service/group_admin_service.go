package service

import (
	"context"
	"di/internal/domain/model"
	"di/internal/transport"
	"net/http"
	"time"
)

// -------------------------- 群管理：管理员设置 / 踢人 / 转让 / 解散 / 退群 --------------------------

// SetAdmin 设置群管理员
func (s *GroupService) SetAdmin(ctx context.Context, operatorUID, groupID, targetUID uint) error {
	// 1. 通用校验：群组有效 + 操作者是群主 + 目标成员是正常群成员
	_, target, err := s.validateAdminOperation(ctx, operatorUID, groupID, targetUID)
	if err != nil {
		return err
	}

	// 2. 设置管理员独有校验
	if target.Role == model.ChatGroupMemberRoleAdmin {
		return transport.NewBizError(http.StatusBadRequest, "target member is already admin")
	}

	adminCount, err := s.groupRepo.CountAdmins(ctx, groupID)
	if err != nil {
		return transport.NewBizError(http.StatusInternalServerError, "count admins failed")
	}
	if adminCount >= 10 {
		return transport.NewBizError(http.StatusBadRequest, "admin limit reached")
	}

	// 3. 执行设置
	target.Role = model.ChatGroupMemberRoleAdmin
	if err := s.groupRepo.UpdateMember(ctx, target); err != nil {
		return transport.NewBizError(http.StatusInternalServerError, "set admin failed")
	}

	return nil
}

// CancelAdmin 取消群管理员
func (s *GroupService) CancelAdmin(ctx context.Context, operatorUID, groupID, targetUID uint) error {
	// 1. 通用校验（完全复用）
	_, target, err := s.validateAdminOperation(ctx, operatorUID, groupID, targetUID)
	if err != nil {
		return err
	}

	// 2. 取消管理员独有校验
	if target.Role != model.ChatGroupMemberRoleAdmin {
		return transport.NewBizError(http.StatusBadRequest, "target member is not admin")
	}

	// 3. 执行取消
	target.Role = model.ChatGroupMemberRoleMember
	if err := s.groupRepo.UpdateMember(ctx, target); err != nil {
		return transport.NewBizError(http.StatusInternalServerError, "cancel admin failed")
	}

	return nil
}

// validateAdminOperation 抽取【设置/取消管理员】的所有公共校验逻辑
func (s *GroupService) validateAdminOperation(ctx context.Context, operatorUID, groupID, targetUID uint) (*model.ChatGroupMember, *model.ChatGroupMember, error) {
	// 1. 校验群组存在且正常
	group, err := s.groupRepo.FindGroupByID(ctx, groupID)
	if err != nil {
		return nil, nil, transport.NewBizError(http.StatusNotFound, "group not found")
	}
	if group.Status != model.ChatGroupStatusNormal {
		return nil, nil, transport.NewBizError(http.StatusBadRequest, "group is not available")
	}

	// 2. 校验操作者是群成员 + 状态正常 + 是群主
	operator, err := s.groupRepo.FindMember(ctx, groupID, operatorUID)
	if err != nil {
		return nil, nil, transport.NewBizError(http.StatusForbidden, "not a group member")
	}
	if operator.Status != model.ChatGroupMemberStatusActive || operator.Role != model.ChatGroupMemberRoleOwner {
		return nil, nil, transport.NewBizError(http.StatusForbidden, "only group owner can manage admin")
	}

	// 3. 校验目标成员存在 + 状态正常
	target, err := s.groupRepo.FindMember(ctx, groupID, targetUID)
	if err != nil {
		return nil, nil, transport.NewBizError(http.StatusNotFound, "target member not found")
	}
	if target.Status != model.ChatGroupMemberStatusActive {
		return nil, nil, transport.NewBizError(http.StatusBadRequest, "target member is not active")
	}

	// 4. 目标不能是群主（两个方法都需要）
	if target.Role == model.ChatGroupMemberRoleOwner {
		return nil, nil, transport.NewBizError(http.StatusBadRequest, "cannot operate owner")
	}

	return operator, target, nil
}

func (s *GroupService) KickMember(ctx context.Context, operatorUID, groupID, targetUID uint) error {
	if operatorUID == targetUID {
		return transport.NewBizError(http.StatusBadRequest, "cannot kick yourself")
	}

	if _, err := s.getNormalGroup(ctx, groupID); err != nil {
		return err
	}

	operator, err := s.requireActiveOperator(ctx, groupID, operatorUID)
	if err != nil {
		return err
	}

	if operator.Role != model.ChatGroupMemberRoleOwner && operator.Role != model.ChatGroupMemberRoleAdmin {
		return transport.NewBizError(http.StatusForbidden, "only owner or admin can kick member")
	}

	target, err := s.getActiveMember(ctx, groupID, targetUID, "target member not found")
	if err != nil {
		return err
	}

	if target.Role == model.ChatGroupMemberRoleOwner {
		return transport.NewBizError(http.StatusBadRequest, "cannot kick group owner")
	}

	if operator.Role == model.ChatGroupMemberRoleAdmin && target.Role != model.ChatGroupMemberRoleMember {
		return transport.NewBizError(http.StatusForbidden, "admin can only kick normal member")
	}

	now := time.Now()
	target.Status = model.ChatGroupMemberStatusKicked
	target.Role = model.ChatGroupMemberRoleMember
	target.LeftAt = &now

	if err := s.groupRepo.UpdateMember(ctx, target); err != nil {
		return transport.NewBizError(http.StatusInternalServerError, "kick member failed")
	}
	// 让被踢成员立即从 active 成员缓存中移除，不再收到群消息推送
	s.refreshMembershipCache(ctx, groupID)

	return nil
}

func (s *GroupService) TransferOwner(ctx context.Context, operatorUID, groupID, targetUID uint) error {
	if operatorUID == targetUID {
		return transport.NewBizError(http.StatusBadRequest, "cannot transfer owner to yourself")
	}

	group, err := s.getNormalGroup(ctx, groupID)
	if err != nil {
		return err
	}

	oldOwner, err := s.requireOwner(ctx, groupID, operatorUID)
	if err != nil {
		return err
	}

	target, err := s.getActiveMember(ctx, groupID, targetUID, "target member not found")
	if err != nil {
		return err
	}

	if target.Role == model.ChatGroupMemberRoleOwner {
		return transport.NewBizError(http.StatusBadRequest, "target is already owner")
	}

	group.OwnerUID = targetUID
	oldOwner.Role = model.ChatGroupMemberRoleAdmin
	target.Role = model.ChatGroupMemberRoleOwner

	if err := s.groupRepo.TransferOwner(ctx, group, oldOwner, target); err != nil {
		return transport.NewBizError(http.StatusInternalServerError, "transfer owner failed")
	}

	return nil
}

func (s *GroupService) DissolveGroup(ctx context.Context, operatorUID, groupID uint) error {
	group, err := s.getNormalGroup(ctx, groupID)
	if err != nil {
		return err
	}

	if _, err := s.requireOwner(ctx, groupID, operatorUID); err != nil {
		return err
	}

	group.Status = model.ChatGroupStatusDissolved

	if err := s.groupRepo.UpdateGroup(ctx, group); err != nil {
		return transport.NewBizError(http.StatusInternalServerError, "dissolve group failed")
	}

	return nil
}

/*
active 成员可以退群
群主不能直接退群，必须转让或解散
退群后 status = left
role 重置为 member
left_at = now
*/
func (s *GroupService) LeaveGroup(ctx context.Context, userID, groupID uint) error {
	if _, err := s.getNormalGroup(ctx, groupID); err != nil {
		return err
	}

	member, err := s.getActiveMember(ctx, groupID, userID, "not a group member")
	if err != nil {
		return err
	}

	if member.Role == model.ChatGroupMemberRoleOwner {
		return transport.NewBizError(http.StatusBadRequest, "owner cannot leave group")
	}

	now := time.Now()
	member.Status = model.ChatGroupMemberStatusLeft
	member.Role = model.ChatGroupMemberRoleMember
	member.LeftAt = &now

	if err := s.groupRepo.UpdateMember(ctx, member); err != nil {
		return transport.NewBizError(http.StatusInternalServerError, "leave group failed")
	}
	s.refreshMembershipCache(ctx, groupID)

	return nil
}
