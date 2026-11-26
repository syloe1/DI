package dao

import (
	"context"
	"time"

	"di/internal/domain/model"
)

type GroupWithMember struct {
	GroupID   uint
	Name      string
	OwnerUID  uint
	Status    string
	Role      string
	JoinedAt  time.Time
	CreatedAt time.Time
}

type GroupRepository interface {
	CreateGroup(ctx context.Context, group *model.ChatGroup, members []model.ChatGroupMember) error
	FindGroupByID(ctx context.Context, groupID uint) (*model.ChatGroup, error)
	UpdateGroup(ctx context.Context, group *model.ChatGroup) error

	FindMember(ctx context.Context, groupID, userID uint) (*model.ChatGroupMember, error)
	ListMembers(ctx context.Context, groupID uint) ([]model.ChatGroupMember, error)
	ListActiveMembers(ctx context.Context, groupID uint) ([]model.ChatGroupMember, error)
	ListGroupsByUserID(ctx context.Context, userID uint) ([]GroupWithMember, error)
	CountAdmins(ctx context.Context, groupID uint) (int64, error)
	UpsertMember(ctx context.Context, member *model.ChatGroupMember) error
	UpdateMember(ctx context.Context, member *model.ChatGroupMember) error

	CreateInvitation(ctx context.Context, invitation *model.ChatGroupInvitation) error
	FindInvitationByID(ctx context.Context, invitationID uint) (*model.ChatGroupInvitation, error)
	FindPendingInvitation(ctx context.Context, groupID, inviteeID uint) (*model.ChatGroupInvitation, error)
	UpdateInvitation(ctx context.Context, invitation *model.ChatGroupInvitation) error

	CreateJoinRequest(ctx context.Context, req *model.ChatGroupJoinRequest) error
	FindJoinRequestByID(ctx context.Context, requestID uint) (*model.ChatGroupJoinRequest, error)
	FindPendingJoinRequest(ctx context.Context, groupID, userID uint) (*model.ChatGroupJoinRequest, error)
	FindLatestRejectedJoinRequest(ctx context.Context, groupID, userID uint) (*model.ChatGroupJoinRequest, error)
	ListJoinRequestsByGroupID(ctx context.Context, groupID uint, limit, offset int) ([]model.ChatGroupJoinRequest, error)
	UpdateJoinRequest(ctx context.Context, req *model.ChatGroupJoinRequest) error

	FindUserByID(ctx context.Context, userID uint) (*model.User, error)
	// FindUsersByIDs 批量查询用户资料（群成员/群消息补 username，避免逐条 N+1）
	FindUsersByIDs(ctx context.Context, userIDs []uint) (map[uint]*model.User, error)
	TransferOwner(ctx context.Context, group *model.ChatGroup, oldOwner *model.ChatGroupMember, newOwner *model.ChatGroupMember) error

	AcceptInvitation(ctx context.Context, invitation *model.ChatGroupInvitation, member *model.ChatGroupMember) error
	ApproveJoinRequest(ctx context.Context, req *model.ChatGroupJoinRequest, member *model.ChatGroupMember) error

	CreateGroupMessage(ctx context.Context, message *model.ChatGroupMessage) error
	CreateGroupMessageWithOutbox(ctx context.Context, message *model.ChatGroupMessage, outbox *model.MessageOutbox) error
	CreateGroupMessageWithOutboxBuilder(ctx context.Context, message *model.ChatGroupMessage, build func(*model.ChatGroupMessage) (*model.MessageOutbox, error)) error
	ListGroupMessages(ctx context.Context, groupID uint, offset int, limit int) ([]model.ChatGroupMessage, error)
	// SyncGroupMessages 群消息断线补偿：拉取 groupID 内 id 大于 afterID 的消息（按 id 升序，limit 条）
	SyncGroupMessages(ctx context.Context, groupID uint, afterID uint, limit int) ([]model.ChatGroupMessage, error)
}
