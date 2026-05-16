package dao

import (
	"go-admin/internal/domain/model"

	"gorm.io/gorm"
)

type GormGroupRepository struct {
	db *gorm.DB
}

func NewGormGroupRepository(db *gorm.DB) *GormGroupRepository {
	return &GormGroupRepository{
		db: db,
	}
}
func (r *GormGroupRepository) CreateGroup(group *model.ChatGroup, members []model.ChatGroupMember) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(group).Error; err != nil {
			return err
		}

		for i := range members {
			members[i].GroupID = group.ID
		}

		if len(members) > 0 {
			if err := tx.Create(&members).Error; err != nil {
				return err
			}
		}

		return nil
	})
}
func (r *GormGroupRepository) FindGroupByID(groupID uint) (*model.ChatGroup, error) {
	var group model.ChatGroup
	if err := r.db.First(&group, groupID).Error; err != nil {
		return nil, err
	}
	return &group, nil
}

func (r *GormGroupRepository) UpdateGroup(group *model.ChatGroup) error {
	return r.db.Save(group).Error
}

func (r *GormGroupRepository) FindMember(groupID, userID uint) (*model.ChatGroupMember, error) {
	var member model.ChatGroupMember
	if err := r.db.Where("group_id = ? AND user_id = ?", groupID, userID).
		First(&member).Error; err != nil {
		return nil, err
	}
	return &member, nil
}

func (r *GormGroupRepository) ListMembers(groupID uint) ([]model.ChatGroupMember, error) {
	var members []model.ChatGroupMember
	if err := r.db.Where("group_id = ?", groupID).
		Order("role ASC, joined_at ASC").
		Find(&members).Error; err != nil {
		return nil, err
	}
	return members, nil
}

func (r *GormGroupRepository) ListActiveMembers(groupID uint) ([]model.ChatGroupMember, error) {
	var members []model.ChatGroupMember
	if err := r.db.Where("group_id = ? AND status = ?", groupID, model.ChatGroupMemberStatusActive).
		Find(&members).Error; err != nil {
		return nil, err
	}
	return members, nil
}

func (r *GormGroupRepository) ListGroupsByUserID(userID uint) ([]GroupWithMember, error) {
	var groups []GroupWithMember
	if err := r.db.Table("chat_groups AS g").
		Select(`
			g.id AS group_id,
			g.name,
			g.owner_uid,
			g.status,
			m.role,
			m.joined_at,
			g.created_at
		`).
		Joins("JOIN chat_group_members AS m ON m.group_id = g.id").
		Where("m.user_id = ? AND m.status = ?", userID, model.ChatGroupMemberStatusActive).
		Where("g.deleted_at IS NULL AND m.deleted_at IS NULL").
		Order("m.joined_at DESC").
		Scan(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func (r *GormGroupRepository) CountAdmins(groupID uint) (int64, error) {
	var count int64
	if err := r.db.Model(&model.ChatGroupMember{}).
		Where("group_id = ? AND role = ? AND status = ?", groupID, model.ChatGroupMemberRoleAdmin, model.ChatGroupMemberStatusActive).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *GormGroupRepository) UpsertMember(member *model.ChatGroupMember) error {
	var existing model.ChatGroupMember
	err := r.db.Where("group_id = ? AND user_id = ?", member.GroupID, member.UserID).First(&existing).Error
	if err == nil {
		member.ID = existing.ID
		return r.db.Model(&existing).Updates(map[string]interface{}{
			"role":      member.Role,
			"status":    member.Status,
			"joined_at": member.JoinedAt,
			"left_at":   member.LeftAt,
		}).Error
	}
	if err != gorm.ErrRecordNotFound {
		return err
	}
	return r.db.Create(member).Error
}

func (r *GormGroupRepository) UpdateMember(member *model.ChatGroupMember) error {
	return r.db.Save(member).Error
}

func (r *GormGroupRepository) CreateInvitation(invitation *model.ChatGroupInvitation) error {
	return r.db.Create(invitation).Error
}

func (r *GormGroupRepository) FindInvitationByID(invitationID uint) (*model.ChatGroupInvitation, error) {
	var invitation model.ChatGroupInvitation
	if err := r.db.First(&invitation, invitationID).Error; err != nil {
		return nil, err
	}
	return &invitation, nil
}

func (r *GormGroupRepository) FindPendingInvitation(groupID, inviteeID uint) (*model.ChatGroupInvitation, error) {
	var invitation model.ChatGroupInvitation
	if err := r.db.Where("group_id = ? AND invitee_uid = ? AND status = ?", groupID, inviteeID, model.ChatGroupInvitationStatusPending).
		First(&invitation).Error; err != nil {
		return nil, err
	}
	return &invitation, nil
}

func (r *GormGroupRepository) UpdateInvitation(invitation *model.ChatGroupInvitation) error {
	return r.db.Save(invitation).Error
}

func (r *GormGroupRepository) CreateJoinRequest(req *model.ChatGroupJoinRequest) error {
	return r.db.Create(req).Error
}

func (r *GormGroupRepository) FindJoinRequestByID(requestID uint) (*model.ChatGroupJoinRequest, error) {
	var req model.ChatGroupJoinRequest
	if err := r.db.First(&req, requestID).Error; err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *GormGroupRepository) FindPendingJoinRequest(groupID, userID uint) (*model.ChatGroupJoinRequest, error) {
	var req model.ChatGroupJoinRequest
	if err := r.db.Where("group_id = ? AND user_id = ? AND status = ?", groupID, userID, model.ChatGroupJoinRequestStatusPending).
		First(&req).Error; err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *GormGroupRepository) FindLatestRejectedJoinRequest(groupID, userID uint) (*model.ChatGroupJoinRequest, error) {
	var req model.ChatGroupJoinRequest
	if err := r.db.Where("group_id = ? AND user_id = ? AND status = ?", groupID, userID, model.ChatGroupJoinRequestStatusRejected).
		Order("created_at DESC").
		First(&req).Error; err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *GormGroupRepository) ListJoinRequestsByGroupID(groupID uint) ([]model.ChatGroupJoinRequest, error) {
	var requests []model.ChatGroupJoinRequest
	if err := r.db.Where("group_id = ?", groupID).
		Order("created_at DESC").
		Find(&requests).Error; err != nil {
		return nil, err
	}
	return requests, nil
}

func (r *GormGroupRepository) UpdateJoinRequest(req *model.ChatGroupJoinRequest) error {
	return r.db.Save(req).Error
}

func (r *GormGroupRepository) FindUserByID(userID uint) (*model.User, error) {
	var user model.User
	if err := r.db.First(&user, userID).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
func (r *GormGroupRepository) TransferOwner(group *model.ChatGroup, oldOwner *model.ChatGroupMember, newOwner *model.ChatGroupMember) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(group).Error; err != nil {
			return err
		}
		if err := tx.Save(oldOwner).Error; err != nil {
			return err
		}
		if err := tx.Save(newOwner).Error; err != nil {
			return err
		}
		return nil
	})
}
func (r *GormGroupRepository) AcceptInvitation(invitation *model.ChatGroupInvitation, member *model.ChatGroupMember) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(invitation).Error; err != nil {
			return err
		}

		var existing model.ChatGroupMember
		err := tx.Where("group_id = ? AND user_id = ?", member.GroupID, member.UserID).First(&existing).Error
		// --- 情况 A：用户已经在群里 ---
		if err == nil {
			member.ID = existing.ID
			return tx.Model(&existing).Updates(map[string]interface{}{
				"role":      member.Role,
				"status":    member.Status,
				"joined_at": member.JoinedAt,
				"left_at":   member.LeftAt,
			}).Error
		}
		// --- 情况 B：查询出错（不是“找不到”） ---
		if err != gorm.ErrRecordNotFound {
			return err
		}

		// --- 情况 C：用户不在群里 → 直接创建新成员 ---
		return tx.Create(member).Error
	})
}
func (r *GormGroupRepository) ApproveJoinRequest(req *model.ChatGroupJoinRequest, member *model.ChatGroupMember) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(req).Error; err != nil {
			return err
		}

		var existing model.ChatGroupMember
		err := tx.Where("group_id = ? AND user_id = ?", member.GroupID, member.UserID).First(&existing).Error
		if err == nil {
			member.ID = existing.ID
			return tx.Model(&existing).Updates(map[string]interface{}{
				"role":      member.Role,
				"status":    member.Status,
				"joined_at": member.JoinedAt,
				"left_at":   member.LeftAt,
			}).Error
		}
		if err != gorm.ErrRecordNotFound {
			return err
		}

		return tx.Create(member).Error
	})
}
func (r *GormGroupRepository) CreateGroupMessage(message *model.ChatGroupMessage) error {
	return r.db.Create(message).Error
}

func (r *GormGroupRepository) ListGroupMessages(groupID uint, offset int, limit int) ([]model.ChatGroupMessage, error) {
	var messages []model.ChatGroupMessage
	err := r.db.Where("group_id = ?", groupID).
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&messages).Error
	return messages, err
}
