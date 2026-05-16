package service

import (
	"go-admin/internal/dao"
	"go-admin/internal/domain/model"
	"go-admin/internal/dto"
	"go-admin/pkg/core"
	"net/http"
	"strings"
	"time"
)

type GroupService struct {
	groupRepo dao.GroupRepository
}

type GroupDetail struct {
	ID        uint      `json:"id"`
	Name      string    `json:"name"`
	OwnerUID  uint      `json:"owner_uid"`
	Status    string    `json:"status"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type GroupMemberItem struct {
	UserID   uint       `json:"user_id"`
	Role     string     `json:"role"`
	Status   string     `json:"status"`
	JoinedAt time.Time  `json:"joined_at"`
	LeftAt   *time.Time `json:"left_at,omitempty"`
}

type MyGroupItem struct {
	ID       uint      `json:"id"`
	Name     string    `json:"name"`
	OwnerUID uint      `json:"owner_uid"`
	Status   string    `json:"status"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

type JoinRequestItem struct {
	ID          uint       `json:"id"`
	GroupID     uint       `json:"group_id"`
	UserID      uint       `json:"user_id"`
	Reason      string     `json:"reason"`
	Status      string     `json:"status"`
	ReviewerUID *uint      `json:"reviewer_uid,omitempty"`
	ReviewedAt  *time.Time `json:"reviewed_at,omitempty"`
	ExpiredAt   time.Time  `json:"expired_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

func NewGroupService(groupRepo dao.GroupRepository) *GroupService {
	return &GroupService{groupRepo: groupRepo}
}

func (s *GroupService) CreateGroup(ownerUID uint, req dto.CreateGroupRequest) (*model.ChatGroup, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, core.NewBizError(http.StatusBadRequest, "group name is required")
	}

	if len(req.MemberIDs) == 0 {
		return nil, core.NewBizError(http.StatusBadRequest, "group must have at least one member")
	}

	memberIDs := make([]uint, 0, len(req.MemberIDs))
	seen := make(map[uint]struct{}, len(req.MemberIDs))

	for _, memberID := range req.MemberIDs {
		if memberID == 0 {
			return nil, core.NewBizError(http.StatusBadRequest, "invalid member id")
		}

		if memberID == ownerUID {
			return nil, core.NewBizError(http.StatusBadRequest, "member_ids cannot contain owner")
		}

		if _, ok := seen[memberID]; ok {
			return nil, core.NewBizError(http.StatusBadRequest, "duplicate member id")
		}

		seen[memberID] = struct{}{}
		memberIDs = append(memberIDs, memberID)
	}

	if _, err := s.groupRepo.FindUserByID(ownerUID); err != nil {
		return nil, core.NewBizError(http.StatusNotFound, "owner user not found")
	}

	for _, memberID := range memberIDs {
		if _, err := s.groupRepo.FindUserByID(memberID); err != nil {
			return nil, core.NewBizError(http.StatusNotFound, "member user not found")
		}
	}

	now := time.Now()

	group := &model.ChatGroup{
		Name:     name,
		OwnerUID: ownerUID,
		Status:   model.ChatGroupStatusNormal,
	}

	members := make([]model.ChatGroupMember, 0, len(memberIDs)+1)
	members = append(members, model.ChatGroupMember{
		UserID:   ownerUID,
		Role:     model.ChatGroupMemberRoleOwner,
		Status:   model.ChatGroupMemberStatusActive,
		JoinedAt: now,
	})

	for _, memberID := range memberIDs {
		members = append(members, model.ChatGroupMember{
			UserID:   memberID,
			Role:     model.ChatGroupMemberRoleMember,
			Status:   model.ChatGroupMemberStatusActive,
			JoinedAt: now,
		})
	}

	if err := s.groupRepo.CreateGroup(group, members); err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "create group failed")
	}

	return group, nil
}

// SetAdmin 设置群管理员
func (s *GroupService) SetAdmin(operatorUID, groupID, targetUID uint) error {
	// 1. 通用校验：群组有效 + 操作者是群主 + 目标成员是正常群成员
	_, target, err := s.validateAdminOperation(operatorUID, groupID, targetUID)
	if err != nil {
		return err
	}

	// 2. 设置管理员独有校验
	if target.Role == model.ChatGroupMemberRoleAdmin {
		return core.NewBizError(http.StatusBadRequest, "target member is already admin")
	}

	adminCount, err := s.groupRepo.CountAdmins(groupID)
	if err != nil {
		return core.NewBizError(http.StatusInternalServerError, "count admins failed")
	}
	if adminCount >= 10 {
		return core.NewBizError(http.StatusBadRequest, "admin limit reached")
	}

	// 3. 执行设置
	target.Role = model.ChatGroupMemberRoleAdmin
	if err := s.groupRepo.UpdateMember(target); err != nil {
		return core.NewBizError(http.StatusInternalServerError, "set admin failed")
	}

	return nil
}

// CancelAdmin 取消群管理员
func (s *GroupService) CancelAdmin(operatorUID, groupID, targetUID uint) error {
	// 1. 通用校验（完全复用）
	_, target, err := s.validateAdminOperation(operatorUID, groupID, targetUID)
	if err != nil {
		return err
	}

	// 2. 取消管理员独有校验
	if target.Role != model.ChatGroupMemberRoleAdmin {
		return core.NewBizError(http.StatusBadRequest, "target member is not admin")
	}

	// 3. 执行取消
	target.Role = model.ChatGroupMemberRoleMember
	if err := s.groupRepo.UpdateMember(target); err != nil {
		return core.NewBizError(http.StatusInternalServerError, "cancel admin failed")
	}

	return nil
}

// -------------------------- 以下是抽取的通用复用方法 --------------------------

// validateAdminOperation 抽取【设置/取消管理员】的所有公共校验逻辑
func (s *GroupService) validateAdminOperation(operatorUID, groupID, targetUID uint) (*model.ChatGroupMember, *model.ChatGroupMember, error) {
	// 1. 校验群组存在且正常
	group, err := s.groupRepo.FindGroupByID(groupID)
	if err != nil {
		return nil, nil, core.NewBizError(http.StatusNotFound, "group not found")
	}
	if group.Status != model.ChatGroupStatusNormal {
		return nil, nil, core.NewBizError(http.StatusBadRequest, "group is not available")
	}

	// 2. 校验操作者是群成员 + 状态正常 + 是群主
	operator, err := s.groupRepo.FindMember(groupID, operatorUID)
	if err != nil {
		return nil, nil, core.NewBizError(http.StatusForbidden, "not a group member")
	}
	if operator.Status != model.ChatGroupMemberStatusActive || operator.Role != model.ChatGroupMemberRoleOwner {
		return nil, nil, core.NewBizError(http.StatusForbidden, "only group owner can manage admin")
	}

	// 3. 校验目标成员存在 + 状态正常
	target, err := s.groupRepo.FindMember(groupID, targetUID)
	if err != nil {
		return nil, nil, core.NewBizError(http.StatusNotFound, "target member not found")
	}
	if target.Status != model.ChatGroupMemberStatusActive {
		return nil, nil, core.NewBizError(http.StatusBadRequest, "target member is not active")
	}

	// 4. 目标不能是群主（两个方法都需要）
	if target.Role == model.ChatGroupMemberRoleOwner {
		return nil, nil, core.NewBizError(http.StatusBadRequest, "cannot operate owner")
	}

	return operator, target, nil
}
func (s *GroupService) getNormalGroup(groupID uint) (*model.ChatGroup, error) {
	group, err := s.groupRepo.FindGroupByID(groupID)
	if err != nil {
		return nil, core.NewBizError(http.StatusNotFound, "group not found")
	}

	if group.Status != model.ChatGroupStatusNormal {
		return nil, core.NewBizError(http.StatusBadRequest, "group is not available")
	}

	return group, nil
}

func (s *GroupService) getActiveMember(groupID, userID uint, notFoundMsg string) (*model.ChatGroupMember, error) {
	member, err := s.groupRepo.FindMember(groupID, userID)
	if err != nil {
		return nil, core.NewBizError(http.StatusNotFound, notFoundMsg)
	}

	if member.Status != model.ChatGroupMemberStatusActive {
		return nil, core.NewBizError(http.StatusBadRequest, notFoundMsg)
	}

	return member, nil
}

func (s *GroupService) requireOwner(groupID, userID uint) (*model.ChatGroupMember, error) {
	member, err := s.getActiveMember(groupID, userID, "not a group member")
	if err != nil {
		return nil, err
	}

	if member.Role != model.ChatGroupMemberRoleOwner {
		return nil, core.NewBizError(http.StatusForbidden, "only group owner can operate")
	}

	return member, nil
}

func (s *GroupService) requireActiveOperator(groupID, userID uint) (*model.ChatGroupMember, error) {
	member, err := s.getActiveMember(groupID, userID, "not a group member")
	if err != nil {
		return nil, err
	}

	return member, nil
}

func (s *GroupService) KickMember(operatorUID, groupID, targetUID uint) error {
	if operatorUID == targetUID {
		return core.NewBizError(http.StatusBadRequest, "cannot kick yourself")
	}

	if _, err := s.getNormalGroup(groupID); err != nil {
		return err
	}

	operator, err := s.requireActiveOperator(groupID, operatorUID)
	if err != nil {
		return err
	}

	if operator.Role != model.ChatGroupMemberRoleOwner && operator.Role != model.ChatGroupMemberRoleAdmin {
		return core.NewBizError(http.StatusForbidden, "only owner or admin can kick member")
	}

	target, err := s.getActiveMember(groupID, targetUID, "target member not found")
	if err != nil {
		return err
	}

	if target.Role == model.ChatGroupMemberRoleOwner {
		return core.NewBizError(http.StatusBadRequest, "cannot kick group owner")
	}

	if operator.Role == model.ChatGroupMemberRoleAdmin && target.Role != model.ChatGroupMemberRoleMember {
		return core.NewBizError(http.StatusForbidden, "admin can only kick normal member")
	}

	now := time.Now()
	target.Status = model.ChatGroupMemberStatusKicked
	target.Role = model.ChatGroupMemberRoleMember
	target.LeftAt = &now

	if err := s.groupRepo.UpdateMember(target); err != nil {
		return core.NewBizError(http.StatusInternalServerError, "kick member failed")
	}

	return nil
}

func (s *GroupService) TransferOwner(operatorUID, groupID, targetUID uint) error {
	if operatorUID == targetUID {
		return core.NewBizError(http.StatusBadRequest, "cannot transfer owner to yourself")
	}

	group, err := s.getNormalGroup(groupID)
	if err != nil {
		return err
	}

	oldOwner, err := s.requireOwner(groupID, operatorUID)
	if err != nil {
		return err
	}

	target, err := s.getActiveMember(groupID, targetUID, "target member not found")
	if err != nil {
		return err
	}

	if target.Role == model.ChatGroupMemberRoleOwner {
		return core.NewBizError(http.StatusBadRequest, "target is already owner")
	}

	group.OwnerUID = targetUID
	oldOwner.Role = model.ChatGroupMemberRoleAdmin
	target.Role = model.ChatGroupMemberRoleOwner

	if err := s.groupRepo.TransferOwner(group, oldOwner, target); err != nil {
		return core.NewBizError(http.StatusInternalServerError, "transfer owner failed")
	}

	return nil
}

func (s *GroupService) DissolveGroup(operatorUID, groupID uint) error {
	group, err := s.getNormalGroup(groupID)
	if err != nil {
		return err
	}

	if _, err := s.requireOwner(groupID, operatorUID); err != nil {
		return err
	}

	group.Status = model.ChatGroupStatusDissolved

	if err := s.groupRepo.UpdateGroup(group); err != nil {
		return core.NewBizError(http.StatusInternalServerError, "dissolve group failed")
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
func (s *GroupService) LeaveGroup(userID, groupID uint) error {
	if _, err := s.getNormalGroup(groupID); err != nil {
		return err
	}

	member, err := s.getActiveMember(groupID, userID, "not a group member")
	if err != nil {
		return err
	}

	if member.Role == model.ChatGroupMemberRoleOwner {
		return core.NewBizError(http.StatusBadRequest, "owner cannot leave group")
	}

	now := time.Now()
	member.Status = model.ChatGroupMemberStatusLeft
	member.Role = model.ChatGroupMemberRoleMember
	member.LeftAt = &now

	if err := s.groupRepo.UpdateMember(member); err != nil {
		return core.NewBizError(http.StatusInternalServerError, "leave group failed")
	}

	return nil
}

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

/*
用户通过 group_id 申请
群 normal
申请理由可选
已在群中不能申请
pending 重复申请返回已有申请
拒绝后 24 小时内不能再次申请
申请有效期 7 天
*/

func (s *GroupService) ApplyJoinGroup(userID, groupID uint, reason string) (*model.ChatGroupJoinRequest, error) {
	if _, err := s.getNormalGroup(groupID); err != nil {
		return nil, err
	}

	if _, err := s.groupRepo.FindUserByID(userID); err != nil {
		return nil, core.NewBizError(http.StatusNotFound, "user not found")
	}

	if member, err := s.groupRepo.FindMember(groupID, userID); err == nil && member.Status == model.ChatGroupMemberStatusActive {
		return nil, core.NewBizError(http.StatusBadRequest, "already in group")
	}

	if req, err := s.groupRepo.FindPendingJoinRequest(groupID, userID); err == nil {
		return req, nil
	}

	if rejected, err := s.groupRepo.FindLatestRejectedJoinRequest(groupID, userID); err == nil {
		if time.Since(rejected.CreatedAt) < 24*time.Hour {
			return nil, core.NewBizError(http.StatusBadRequest, "can apply again after 24 hours")
		}
	}

	reason = strings.TrimSpace(reason)
	if len(reason) > 255 {
		return nil, core.NewBizError(http.StatusBadRequest, "reason is too long")
	}

	req := &model.ChatGroupJoinRequest{
		GroupID:   groupID,
		UserID:    userID,
		Reason:    reason,
		Status:    model.ChatGroupJoinRequestStatusPending,
		ExpiredAt: time.Now().Add(7 * 24 * time.Hour),
	}

	if err := s.groupRepo.CreateJoinRequest(req); err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "create join request failed")
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
func (s *GroupService) ReviewJoinRequest(operatorUID, requestID uint, action string) error {
	req, err := s.groupRepo.FindJoinRequestByID(requestID)
	if err != nil {
		return core.NewBizError(http.StatusNotFound, "join request not found")
	}

	if req.Status != model.ChatGroupJoinRequestStatusPending {
		return core.NewBizError(http.StatusBadRequest, "join request is not pending")
	}

	if _, err := s.getNormalGroup(req.GroupID); err != nil {
		return err
	}

	operator, err := s.getActiveMember(req.GroupID, operatorUID, "not a group member")
	if err != nil {
		return err
	}

	if operator.Role != model.ChatGroupMemberRoleOwner && operator.Role != model.ChatGroupMemberRoleAdmin {
		return core.NewBizError(http.StatusForbidden, "only owner or admin can review join request")
	}

	now := time.Now()
	if now.After(req.ExpiredAt) {
		req.Status = model.ChatGroupJoinRequestStatusExpired
		req.ReviewerUID = &operatorUID
		req.ReviewedAt = &now
		_ = s.groupRepo.UpdateJoinRequest(req)
		return core.NewBizError(http.StatusBadRequest, "join request expired")
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

		if err := s.groupRepo.ApproveJoinRequest(req, member); err != nil {
			return core.NewBizError(http.StatusInternalServerError, "approve join request failed")
		}

	case "reject":
		req.Status = model.ChatGroupJoinRequestStatusRejected

		if err := s.groupRepo.UpdateJoinRequest(req); err != nil {
			return core.NewBizError(http.StatusInternalServerError, "reject join request failed")
		}

	default:
		return core.NewBizError(http.StatusBadRequest, "invalid action")
	}

	return nil
}

func (s *GroupService) GetGroupDetail(userID, groupID uint) (*GroupDetail, error) {
	group, err := s.groupRepo.FindGroupByID(groupID)
	if err != nil {
		return nil, core.NewBizError(http.StatusNotFound, "group not found")
	}

	member, err := s.getActiveMember(groupID, userID, "not a group member")
	if err != nil {
		return nil, err
	}

	return &GroupDetail{
		ID:        group.ID,
		Name:      group.Name,
		OwnerUID:  group.OwnerUID,
		Status:    group.Status,
		Role:      member.Role,
		CreatedAt: group.CreatedAt,
	}, nil
}

func (s *GroupService) GetGroupMembers(userID, groupID uint) ([]GroupMemberItem, error) {
	if _, err := s.getActiveMember(groupID, userID, "not a group member"); err != nil {
		return nil, err
	}

	members, err := s.groupRepo.ListMembers(groupID)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get group members failed")
	}

	result := make([]GroupMemberItem, 0, len(members))
	for _, member := range members {
		result = append(result, GroupMemberItem{
			UserID:   member.UserID,
			Role:     member.Role,
			Status:   member.Status,
			JoinedAt: member.JoinedAt,
			LeftAt:   member.LeftAt,
		})
	}

	return result, nil
}

func (s *GroupService) GetMyGroups(userID uint) ([]MyGroupItem, error) {
	groups, err := s.groupRepo.ListGroupsByUserID(userID)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get my groups failed")
	}

	result := make([]MyGroupItem, 0, len(groups))
	for _, group := range groups {
		result = append(result, MyGroupItem{
			ID:       group.GroupID,
			Name:     group.Name,
			OwnerUID: group.OwnerUID,
			Status:   group.Status,
			Role:     group.Role,
			JoinedAt: group.JoinedAt,
		})
	}

	return result, nil
}

func (s *GroupService) GetJoinRequests(userID, groupID uint) ([]JoinRequestItem, error) {
	if _, err := s.getNormalGroup(groupID); err != nil {
		return nil, err
	}

	operator, err := s.getActiveMember(groupID, userID, "not a group member")
	if err != nil {
		return nil, err
	}

	if operator.Role != model.ChatGroupMemberRoleOwner && operator.Role != model.ChatGroupMemberRoleAdmin {
		return nil, core.NewBizError(http.StatusForbidden, "only owner or admin can view join requests")
	}

	requests, err := s.groupRepo.ListJoinRequestsByGroupID(groupID)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get join requests failed")
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
func (s *GroupService) SendGroupMessage(userID, groupID uint, content string) (*model.ChatGroupMessage, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, core.NewBizError(http.StatusBadRequest, "content is required")
	}
	if len(content) > 2000 {
		return nil, core.NewBizError(http.StatusBadRequest, "content is too long")
	}

	if _, err := s.getNormalGroup(groupID); err != nil {
		return nil, err
	}

	if _, err := s.getActiveMember(groupID, userID, "not a group member"); err != nil {
		return nil, err
	}

	message := &model.ChatGroupMessage{
		GroupID:   groupID,
		SenderUID: userID,
		Content:   content,
	}

	if err := s.groupRepo.CreateGroupMessage(message); err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "send group message failed")
	}

	return message, nil
}
func (s *GroupService) GetGroupMessages(userID, groupID uint, page, pageSize int) ([]model.ChatGroupMessage, error) {
	page, pageSize = normalizePagination(page, pageSize, 1, 20)
	offset := (page - 1) * pageSize

	if _, err := s.getNormalGroup(groupID); err != nil {
		return nil, err
	}

	if _, err := s.getActiveMember(groupID, userID, "not a group member"); err != nil {
		return nil, err
	}

	messages, err := s.groupRepo.ListGroupMessages(groupID, offset, pageSize)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get group messages failed")
	}

	return messages, nil
}
