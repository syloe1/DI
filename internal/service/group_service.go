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

// -------------------------- 公共复用方法（群组域各子模块共用） --------------------------

// getNormalGroup 校验群组存在且状态 normal。
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

// getActiveMember 校验用户是正常（active）群成员。
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

// requireOwner 校验操作者是群主。
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

// requireActiveOperator 校验操作者是正常成员（不限定角色）。
func (s *GroupService) requireActiveOperator(groupID, userID uint) (*model.ChatGroupMember, error) {
	member, err := s.getActiveMember(groupID, userID, "not a group member")
	if err != nil {
		return nil, err
	}

	return member, nil
}

// -------------------------- 查询 --------------------------

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
