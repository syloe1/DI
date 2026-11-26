package dao

import (
	"context"
	"fmt"

	"di/internal/domain/model"

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

func (r *GormGroupRepository) CreateGroup(ctx context.Context, group *model.ChatGroup, members []model.ChatGroupMember) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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

func (r *GormGroupRepository) FindGroupByID(ctx context.Context, groupID uint) (*model.ChatGroup, error) {
	var group model.ChatGroup
	if err := r.db.WithContext(ctx).First(&group, groupID).Error; err != nil {
		return nil, err
	}
	return &group, nil
}

func (r *GormGroupRepository) UpdateGroup(ctx context.Context, group *model.ChatGroup) error {
	return r.db.WithContext(ctx).Save(group).Error
}

func (r *GormGroupRepository) FindMember(ctx context.Context, groupID, userID uint) (*model.ChatGroupMember, error) {
	var member model.ChatGroupMember
	if err := r.db.WithContext(ctx).Where("group_id = ? AND user_id = ?", groupID, userID).
		First(&member).Error; err != nil {
		return nil, err
	}
	return &member, nil
}

func (r *GormGroupRepository) ListMembers(ctx context.Context, groupID uint) ([]model.ChatGroupMember, error) {
	var members []model.ChatGroupMember
	if err := r.db.WithContext(ctx).Where("group_id = ?", groupID).
		Order("role ASC, joined_at ASC").
		Find(&members).Error; err != nil {
		return nil, err
	}
	return members, nil
}

func (r *GormGroupRepository) ListActiveMembers(ctx context.Context, groupID uint) ([]model.ChatGroupMember, error) {
	var members []model.ChatGroupMember
	if err := r.db.WithContext(ctx).Where("group_id = ? AND status = ?", groupID, model.ChatGroupMemberStatusActive).
		Find(&members).Error; err != nil {
		return nil, err
	}
	return members, nil
}

func (r *GormGroupRepository) ListGroupsByUserID(ctx context.Context, userID uint) ([]GroupWithMember, error) {
	var groups []GroupWithMember
	if err := r.db.WithContext(ctx).Table("chat_groups AS g").
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
		Where("g.status = ?", model.ChatGroupStatusNormal).
		Where("g.deleted_at IS NULL AND m.deleted_at IS NULL").
		Order("m.joined_at DESC").
		Scan(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func (r *GormGroupRepository) CountAdmins(ctx context.Context, groupID uint) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.ChatGroupMember{}).
		Where("group_id = ? AND role = ? AND status = ?", groupID, model.ChatGroupMemberRoleAdmin, model.ChatGroupMemberStatusActive).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *GormGroupRepository) UpsertMember(ctx context.Context, member *model.ChatGroupMember) error {
	db := r.db.WithContext(ctx)
	var existing model.ChatGroupMember
	err := db.Where("group_id = ? AND user_id = ?", member.GroupID, member.UserID).First(&existing).Error
	if err == nil {
		member.ID = existing.ID
		return db.Model(&existing).Updates(map[string]interface{}{
			"role":      member.Role,
			"status":    member.Status,
			"joined_at": member.JoinedAt,
			"left_at":   member.LeftAt,
		}).Error
	}
	if err != gorm.ErrRecordNotFound {
		return err
	}
	return db.Create(member).Error
}

func (r *GormGroupRepository) UpdateMember(ctx context.Context, member *model.ChatGroupMember) error {
	return r.db.WithContext(ctx).Save(member).Error
}

func (r *GormGroupRepository) CreateInvitation(ctx context.Context, invitation *model.ChatGroupInvitation) error {
	return r.db.WithContext(ctx).Create(invitation).Error
}

func (r *GormGroupRepository) FindInvitationByID(ctx context.Context, invitationID uint) (*model.ChatGroupInvitation, error) {
	var invitation model.ChatGroupInvitation
	if err := r.db.WithContext(ctx).First(&invitation, invitationID).Error; err != nil {
		return nil, err
	}
	return &invitation, nil
}

func (r *GormGroupRepository) FindPendingInvitation(ctx context.Context, groupID, inviteeID uint) (*model.ChatGroupInvitation, error) {
	var invitation model.ChatGroupInvitation
	if err := r.db.WithContext(ctx).Where("group_id = ? AND invitee_uid = ? AND status = ?", groupID, inviteeID, model.ChatGroupInvitationStatusPending).
		First(&invitation).Error; err != nil {
		return nil, err
	}
	return &invitation, nil
}

func (r *GormGroupRepository) UpdateInvitation(ctx context.Context, invitation *model.ChatGroupInvitation) error {
	return r.db.WithContext(ctx).Save(invitation).Error
}

func (r *GormGroupRepository) CreateJoinRequest(ctx context.Context, req *model.ChatGroupJoinRequest) error {
	return r.db.WithContext(ctx).Create(req).Error
}

func (r *GormGroupRepository) FindJoinRequestByID(ctx context.Context, requestID uint) (*model.ChatGroupJoinRequest, error) {
	var req model.ChatGroupJoinRequest
	if err := r.db.WithContext(ctx).First(&req, requestID).Error; err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *GormGroupRepository) FindPendingJoinRequest(ctx context.Context, groupID, userID uint) (*model.ChatGroupJoinRequest, error) {
	var req model.ChatGroupJoinRequest
	if err := r.db.WithContext(ctx).Where("group_id = ? AND user_id = ? AND status = ?", groupID, userID, model.ChatGroupJoinRequestStatusPending).
		First(&req).Error; err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *GormGroupRepository) FindLatestRejectedJoinRequest(ctx context.Context, groupID, userID uint) (*model.ChatGroupJoinRequest, error) {
	var req model.ChatGroupJoinRequest
	if err := r.db.WithContext(ctx).Where("group_id = ? AND user_id = ? AND status = ?", groupID, userID, model.ChatGroupJoinRequestStatusRejected).
		Order("created_at DESC").
		First(&req).Error; err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *GormGroupRepository) ListJoinRequestsByGroupID(ctx context.Context, groupID uint, limit, offset int) ([]model.ChatGroupJoinRequest, error) {
	var requests []model.ChatGroupJoinRequest
	if err := r.db.WithContext(ctx).Where("group_id = ?", groupID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&requests).Error; err != nil {
		return nil, err
	}
	return requests, nil
}

func (r *GormGroupRepository) UpdateJoinRequest(ctx context.Context, req *model.ChatGroupJoinRequest) error {
	return r.db.WithContext(ctx).Save(req).Error
}

func (r *GormGroupRepository) FindUserByID(ctx context.Context, userID uint) (*model.User, error) {
	var user model.User
	if err := r.db.WithContext(ctx).First(&user, userID).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// FindUsersByIDs 批量查用户，返回 id -> *User（一次 IN 查询替代 N 次单查）。
func (r *GormGroupRepository) FindUsersByIDs(ctx context.Context, userIDs []uint) (map[uint]*model.User, error) {
	result := make(map[uint]*model.User, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}

	var users []model.User
	if err := r.db.WithContext(ctx).Where("id IN ?", userIDs).Find(&users).Error; err != nil {
		return nil, err
	}
	for i := range users {
		result[users[i].ID] = &users[i]
	}
	return result, nil
}

func (r *GormGroupRepository) TransferOwner(ctx context.Context, group *model.ChatGroup, oldOwner *model.ChatGroupMember, newOwner *model.ChatGroupMember) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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

func (r *GormGroupRepository) AcceptInvitation(ctx context.Context, invitation *model.ChatGroupInvitation, member *model.ChatGroupMember) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(invitation).Error; err != nil {
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

func (r *GormGroupRepository) ApproveJoinRequest(ctx context.Context, req *model.ChatGroupJoinRequest, member *model.ChatGroupMember) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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

func (r *GormGroupRepository) CreateGroupMessage(ctx context.Context, message *model.ChatGroupMessage) error {
	return r.db.WithContext(ctx).Create(message).Error
}

func (r *GormGroupRepository) CreateGroupMessageWithOutbox(ctx context.Context, message *model.ChatGroupMessage, outbox *model.MessageOutbox) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(message).Error; err != nil {
			return err
		}
		if outbox != nil {
			outbox.AggregateID = fmt.Sprintf("%d", message.ID)
			if err := tx.Create(outbox).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *GormGroupRepository) CreateGroupMessageWithOutboxBuilder(ctx context.Context, message *model.ChatGroupMessage, build func(*model.ChatGroupMessage) (*model.MessageOutbox, error)) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(message).Error; err != nil {
			return err
		}
		outbox, err := build(message)
		if err != nil {
			return err
		}
		if outbox != nil {
			outbox.AggregateID = fmt.Sprintf("%d", message.ID)
			if err := tx.Create(outbox).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *GormGroupRepository) ListGroupMessages(ctx context.Context, groupID uint, offset int, limit int) ([]model.ChatGroupMessage, error) {
	var messages []model.ChatGroupMessage
	db := r.db.WithContext(ctx)
	latest := db.Model(&model.ChatGroupMessage{}).
		Where("group_id = ?", groupID).
		Order("created_at DESC, id DESC").
		Offset(offset).
		Limit(limit)
	err := db.Table("(?) AS latest", latest).
		Order("created_at ASC, id ASC").
		Find(&messages).Error
	return messages, err
}

// SyncGroupMessages 群消息断线补偿：拉取 groupID 内 id 大于 afterID 的消息，按 id 升序最多 limit 条。
func (r *GormGroupRepository) SyncGroupMessages(ctx context.Context, groupID uint, afterID uint, limit int) ([]model.ChatGroupMessage, error) {
	var messages []model.ChatGroupMessage
	if err := r.db.WithContext(ctx).Model(&model.ChatGroupMessage{}).
		Where("group_id = ? AND id > ?", groupID, afterID).
		Order("id ASC").
		Limit(limit).
		Find(&messages).Error; err != nil {
		return nil, err
	}
	return messages, nil
}
