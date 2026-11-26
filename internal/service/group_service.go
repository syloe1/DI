package service

import (
	"context"
	"di/internal/dao"
	"di/internal/domain/model"
	"di/internal/dto"
	"di/internal/telemetry"
	"di/internal/transport"
	"net/http"
	"strings"
	"time"
)

type GroupService struct {
	groupRepo  dao.GroupRepository
	groupCache *GroupCacheService // 成员变更后刷新 active 成员缓存（可为 nil）
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
	Username string     `json:"username,omitempty"` // 后端直接带 username，前端无需逐条补查
	Role     string     `json:"role"`
	Status   string     `json:"status"`
	JoinedAt time.Time  `json:"joined_at"`
	LeftAt   *time.Time `json:"left_at,omitempty"`
}

// GroupMessageItem 群消息响应 DTO：直接带发送者 username，
// 避免前端拿到 sender_uid 后再逐个 /user/{id} 补查（N+1），
// 也保证刷新页面后发送者名称与实时推送一致。
type GroupMessageItem struct {
	ID        uint      `json:"id"`
	GroupID   uint      `json:"group_id"`
	SenderUID uint      `json:"sender_uid"`
	Username  string    `json:"username,omitempty"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
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

func NewGroupService(groupRepo dao.GroupRepository, groupCache *GroupCacheService) *GroupService {
	return &GroupService{groupRepo: groupRepo, groupCache: groupCache}
}

// refreshMembershipCache 成员状态变化后刷新「群 active 成员」缓存。
// 否则被踢/退群用户在缓存 TTL（10 分钟）内仍可能收到群消息推送。
func (s *GroupService) refreshMembershipCache(ctx context.Context, groupID uint) {
	if s == nil || s.groupCache == nil || groupID == 0 {
		return
	}
	if err := s.groupCache.RefreshActiveMembers(context.Background(), groupID); err != nil {
		telemetry.Z().Warnf("refresh group member cache failed group_id=%d: %v", groupID, err)
	}
}

func (s *GroupService) CreateGroup(ctx context.Context, ownerUID uint, req dto.CreateGroupRequest) (*model.ChatGroup, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, transport.NewBizError(http.StatusBadRequest, "group name is required")
	}

	if len(req.MemberIDs) == 0 {
		return nil, transport.NewBizError(http.StatusBadRequest, "group must have at least one member")
	}

	memberIDs := make([]uint, 0, len(req.MemberIDs))
	seen := make(map[uint]struct{}, len(req.MemberIDs))

	for _, memberID := range req.MemberIDs {
		if memberID == 0 {
			return nil, transport.NewBizError(http.StatusBadRequest, "invalid member id")
		}

		if memberID == ownerUID {
			return nil, transport.NewBizError(http.StatusBadRequest, "member_ids cannot contain owner")
		}

		if _, ok := seen[memberID]; ok {
			return nil, transport.NewBizError(http.StatusBadRequest, "duplicate member id")
		}

		seen[memberID] = struct{}{}
		memberIDs = append(memberIDs, memberID)
	}

	// 一次性批量校验 owner + 所有成员是否存在（替代逐个 FindUserByID 的 N+1）
	allIDs := make([]uint, 0, len(memberIDs)+1)
	allIDs = append(allIDs, ownerUID)
	allIDs = append(allIDs, memberIDs...)
	userMap, err := s.groupRepo.FindUsersByIDs(ctx, allIDs)
	if err != nil {
		return nil, transport.NewBizError(http.StatusInternalServerError, "query users failed")
	}
	if _, ok := userMap[ownerUID]; !ok {
		return nil, transport.NewBizError(http.StatusNotFound, "owner user not found")
	}
	for _, memberID := range memberIDs {
		if _, ok := userMap[memberID]; !ok {
			return nil, transport.NewBizError(http.StatusNotFound, "member user not found")
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

	if err := s.groupRepo.CreateGroup(ctx, group, members); err != nil {
		return nil, transport.NewBizError(http.StatusInternalServerError, "create group failed")
	}

	return group, nil
}

// -------------------------- 公共复用方法（群组域各子模块共用） --------------------------

// getNormalGroup 校验群组存在且状态 normal；已解散返回 410（Gone），便于前端区分展示。
func (s *GroupService) getNormalGroup(ctx context.Context, groupID uint) (*model.ChatGroup, error) {
	group, err := s.groupRepo.FindGroupByID(ctx, groupID)
	if err != nil {
		return nil, transport.NewBizError(http.StatusNotFound, "group not found")
	}

	if group.Status != model.ChatGroupStatusNormal {
		return nil, transport.NewBizError(http.StatusGone, "group is dissolved")
	}

	return group, nil
}

// getActiveMember 校验用户是正常（active）群成员。
func (s *GroupService) getActiveMember(ctx context.Context, groupID, userID uint, notFoundMsg string) (*model.ChatGroupMember, error) {
	member, err := s.groupRepo.FindMember(ctx, groupID, userID)
	if err != nil {
		return nil, transport.NewBizError(http.StatusNotFound, notFoundMsg)
	}

	if member.Status != model.ChatGroupMemberStatusActive {
		return nil, transport.NewBizError(http.StatusBadRequest, notFoundMsg)
	}

	return member, nil
}

// requireOwner 校验操作者是群主。
func (s *GroupService) requireOwner(ctx context.Context, groupID, userID uint) (*model.ChatGroupMember, error) {
	member, err := s.getActiveMember(ctx, groupID, userID, "not a group member")
	if err != nil {
		return nil, err
	}

	if member.Role != model.ChatGroupMemberRoleOwner {
		return nil, transport.NewBizError(http.StatusForbidden, "only group owner can operate")
	}

	return member, nil
}

// requireActiveOperator 校验操作者是正常成员（不限定角色）。
func (s *GroupService) requireActiveOperator(ctx context.Context, groupID, userID uint) (*model.ChatGroupMember, error) {
	member, err := s.getActiveMember(ctx, groupID, userID, "not a group member")
	if err != nil {
		return nil, err
	}

	return member, nil
}

// -------------------------- 查询 --------------------------

func (s *GroupService) GetGroupDetail(ctx context.Context, userID, groupID uint) (*GroupDetail, error) {
	group, err := s.groupRepo.FindGroupByID(ctx, groupID)
	if err != nil {
		return nil, transport.NewBizError(http.StatusNotFound, "group not found")
	}

	// 已解散/归档的群不再提供详情（404 语义之外用 410 明确告知「已不存在」）
	if group.Status != model.ChatGroupStatusNormal {
		return nil, transport.NewBizError(http.StatusGone, "group is dissolved")
	}

	member, err := s.getActiveMember(ctx, groupID, userID, "not a group member")
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

func (s *GroupService) GetGroupMembers(ctx context.Context, userID, groupID uint) ([]GroupMemberItem, error) {
	if _, err := s.getActiveMember(ctx, groupID, userID, "not a group member"); err != nil {
		return nil, err
	}

	// 只返回 active 成员：被踢/主动退群/已离开的成员不应出现在「群成员」面板里
	members, err := s.groupRepo.ListActiveMembers(ctx, groupID)
	if err != nil {
		return nil, transport.NewBizError(http.StatusInternalServerError, "get group members failed")
	}

	// 批量补 username（一次 IN 查询，替代前端逐成员 /user/{id} 的 N+1）
	userIDs := make([]uint, 0, len(members))
	seen := make(map[uint]struct{}, len(members))
	for _, member := range members {
		if _, ok := seen[member.UserID]; ok {
			continue
		}
		seen[member.UserID] = struct{}{}
		userIDs = append(userIDs, member.UserID)
	}
	userMap, err := s.groupRepo.FindUsersByIDs(ctx, userIDs)
	if err != nil {
		// 补名字失败不影响成员列表主流程，前端还有「用户{id}」兜底；记录告警便于定位
		telemetry.Z().Warnf("load member usernames failed group_id=%d: %v", groupID, err)
		userMap = map[uint]*model.User{}
	}

	result := make([]GroupMemberItem, 0, len(members))
	for _, member := range members {
		username := ""
		if user, ok := userMap[member.UserID]; ok {
			username = user.Username
		}
		result = append(result, GroupMemberItem{
			UserID:   member.UserID,
			Username: username,
			Role:     member.Role,
			Status:   member.Status,
			JoinedAt: member.JoinedAt,
			LeftAt:   member.LeftAt,
		})
	}

	return result, nil
}

func (s *GroupService) GetMyGroups(ctx context.Context, userID uint) ([]MyGroupItem, error) {
	groups, err := s.groupRepo.ListGroupsByUserID(ctx, userID)
	if err != nil {
		return nil, transport.NewBizError(http.StatusInternalServerError, "get my groups failed")
	}

	result := make([]MyGroupItem, 0, len(groups))
	for _, group := range groups {
		// 双保险：DAO 已过滤 normal，这里再挡一次，确保解散群不会回到普通列表
		if group.Status != model.ChatGroupStatusNormal {
			continue
		}
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
