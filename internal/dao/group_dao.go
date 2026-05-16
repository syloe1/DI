package dao

import (
	"time"

	"go-admin/internal/domain/model"
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
	CreateGroup(group *model.ChatGroup, members []model.ChatGroupMember) error
	FindGroupByID(groupID uint) (*model.ChatGroup, error)
	UpdateGroup(group *model.ChatGroup) error

	FindMember(groupID, userID uint) (*model.ChatGroupMember, error)
	ListMembers(groupID uint) ([]model.ChatGroupMember, error)
	ListActiveMembers(groupID uint) ([]model.ChatGroupMember, error)
	ListGroupsByUserID(userID uint) ([]GroupWithMember, error)
	CountAdmins(groupID uint) (int64, error)
	UpsertMember(member *model.ChatGroupMember) error
	UpdateMember(member *model.ChatGroupMember) error

	CreateInvitation(invitation *model.ChatGroupInvitation) error
	FindInvitationByID(invitationID uint) (*model.ChatGroupInvitation, error)
	FindPendingInvitation(groupID, inviteeID uint) (*model.ChatGroupInvitation, error)
	UpdateInvitation(invitation *model.ChatGroupInvitation) error

	CreateJoinRequest(req *model.ChatGroupJoinRequest) error
	FindJoinRequestByID(requestID uint) (*model.ChatGroupJoinRequest, error)
	FindPendingJoinRequest(groupID, userID uint) (*model.ChatGroupJoinRequest, error)
	FindLatestRejectedJoinRequest(groupID, userID uint) (*model.ChatGroupJoinRequest, error)
	ListJoinRequestsByGroupID(groupID uint) ([]model.ChatGroupJoinRequest, error)
	UpdateJoinRequest(req *model.ChatGroupJoinRequest) error

	FindUserByID(userID uint) (*model.User, error)
	TransferOwner(group *model.ChatGroup, oldOwner *model.ChatGroupMember, newOwner *model.ChatGroupMember) error

	AcceptInvitation(invitation *model.ChatGroupInvitation, member *model.ChatGroupMember) error
	ApproveJoinRequest(req *model.ChatGroupJoinRequest, member *model.ChatGroupMember) error

	CreateGroupMessage(message *model.ChatGroupMessage) error
	ListGroupMessages(groupID uint, offset int, limit int) ([]model.ChatGroupMessage, error)
}
