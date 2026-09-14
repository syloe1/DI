package dao

import (
	"go-admin/internal/domain/model"

	"gorm.io/gorm"
)

// MessageRepository 定义消息相关的数据库操作
type MessageRepository interface {
	// 消息相关方法
	CreateMessage(message *model.Message) error
	FindMessageByID(id uint) (*model.Message, error)
	GetConversations(userID uint) ([]Conversation, error)
	GetMessages(userID uint, peerID uint, offset int, limit int) ([]model.Message, error)
	MarkMessagesAsRead(userID uint, peerID uint) error
	CountUnreadMessages(userID uint, peerID uint) (int64, error)
	// N+1 修复：批量查询接口（替代循环内单查）
	FindUsersByIDs(ids []uint) (map[uint]*model.User, error)
	CountUnreadByPeers(userID uint, peerIDs []uint) (map[uint]int64, error)
	// 断线补偿：增量拉取 id 大于 afterID 的消息（按 id 升序，limit 条）
	SyncMessages(userID uint, afterID uint, limit int) ([]model.Message, error)
	// 已读回执：查询 peer 发给 userID 的未读消息 ID（用于标记已读后推送回执）
	FindUnreadMessageIDs(userID uint, peerID uint) ([]uint, error)
	DeleteMessage(message *model.Message) error

	// 用户相关方法
	FindUserByID(userID uint) (*model.User, error)
}

// GormMessageRepository 基于 GORM 的消息 Repository 实现
type GormMessageRepository struct {
	db *gorm.DB
}

// NewGormMessageRepository 构造函数：注入 DB 依赖
func NewGormMessageRepository(db *gorm.DB) *GormMessageRepository {
	return &GormMessageRepository{db: db}
}

// 消息相关方法

// CreateMessage 创建消息
func (r *GormMessageRepository) CreateMessage(message *model.Message) error {
	return r.db.Create(message).Error
}

// FindMessageByID 根据 ID 查询消息
func (r *GormMessageRepository) FindMessageByID(id uint) (*model.Message, error) {
	var message model.Message
	//select * from messages where id = ? limit 1
	if err := r.db.First(&message, id).Error; err != nil {
		return nil, err
	}
	return &message, nil
}

// GetConversations 获取用户的会话列表
func (r *GormMessageRepository) GetConversations(userID uint) ([]Conversation, error) {
	var conversations []Conversation

	// 使用原生 SQL 查询会话列表
	sql := `
		SELECT 
			CASE WHEN from_uid = ? THEN to_uid ELSE from_uid END AS peer_uid,
			content AS last_msg,
			created_at AS last_time,
			0 AS unread
		FROM messages 
		WHERE id IN (
			SELECT MAX(id) FROM messages 
			WHERE (from_uid = ? OR to_uid = ?) 
			AND deleted_at IS NULL
			GROUP BY CASE WHEN from_uid = ? THEN to_uid ELSE from_uid END
		)
		AND deleted_at IS NULL
		ORDER BY created_at DESC
	`

	if err := r.db.Raw(sql, userID, userID, userID, userID).Scan(&conversations).Error; err != nil {
		return nil, err
	}

	return conversations, nil
}

// GetMessages 分页查询两个用户之间的聊天记录
func (r *GormMessageRepository) GetMessages(userID uint, peerID uint, offset int, limit int) ([]model.Message, error) {
	var messages []model.Message

	if err := r.db.Where("(from_uid = ? AND to_uid = ?) OR (from_uid = ? AND to_uid = ?)",
		userID, peerID, peerID, userID).
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&messages).Error; err != nil {
		return nil, err
	}

	return messages, nil
}

// MarkMessagesAsRead 将对方发给当前用户的消息标记为已读
func (r *GormMessageRepository) MarkMessagesAsRead(userID uint, peerID uint) error {
	/*
		UPDATE messages
		SET is_read = true
		WHERE
		    from_uid = 对方    -- 对方发给我
		AND to_uid = 我        -- 我是接收者
		AND is_read = false;   -- 只更新未读的
	*/
	return r.db.Model(&model.Message{}).
		Where("from_uid = ? AND to_uid = ? AND is_read = false", peerID, userID).
		Update("is_read", true).Error
}

// CountUnreadMessages 统计未读消息数量
func (r *GormMessageRepository) CountUnreadMessages(userID uint, peerID uint) (int64, error) {
	var count int64
	/*
	   SELECT COUNT(*) FROM messages
	   WHERE
	       from_uid = 对方
	   AND to_uid = 我
	   AND is_read = false;
	*/
	if err := r.db.Model(&model.Message{}).
		Where("from_uid = ? AND to_uid = ? AND is_read = false", peerID, userID).
		Count(&count).Error; err != nil {
		return 0, err
	}

	return count, nil
}

// FindUsersByIDs 批量查询用户（N+1 修复：一次 IN 查询替代循环 FindUserByID）。
// 返回 map[userID]*User，不存在的 ID 不在 map 中。
func (r *GormMessageRepository) FindUsersByIDs(ids []uint) (map[uint]*model.User, error) {
	var users []model.User
	if err := r.db.Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	result := make(map[uint]*model.User, len(users))
	for i := range users {
		result[users[i].ID] = &users[i]
	}
	return result, nil
}

// CountUnreadByPeers 批量统计 peerIDs 中每个用户发给 userID 的未读数（N+1 修复：一次 GROUP BY 替代循环 CountUnreadMessages）。
// 返回 map[fromUID]unreadCount，没有未读的 peer 不在 map 中（读为 0）。
func (r *GormMessageRepository) CountUnreadByPeers(userID uint, peerIDs []uint) (map[uint]int64, error) {
	type unreadRow struct {
		FromUID uint
		Count   int64
	}
	var rows []unreadRow
	// GORM 的 Model(&model.Message{}) 会自动附加 deleted_at IS NULL（软删除）
	if err := r.db.Model(&model.Message{}).
		Select("from_uid, COUNT(*) AS count").
		Where("to_uid = ? AND from_uid IN ? AND is_read = false", userID, peerIDs).
		Group("from_uid").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[uint]int64, len(rows))
	for _, row := range rows {
		result[row.FromUID] = row.Count
	}
	return result, nil
}

// SyncMessages 增量拉取：获取 id 大于 afterID、且与 userID 相关的消息（断线补偿用）。
// 按 id 升序返回最多 limit 条；客户端用返回的最后一条 id 作为下一次游标继续拉。
func (r *GormMessageRepository) SyncMessages(userID uint, afterID uint, limit int) ([]model.Message, error) {
	var messages []model.Message
	if err := r.db.
		Where("id > ? AND (from_uid = ? OR to_uid = ?)", afterID, userID, userID).
		Order("id ASC").
		Limit(limit).
		Find(&messages).Error; err != nil {
		return nil, err
	}
	return messages, nil
}

// FindUnreadMessageIDs 查询 peerID 发给 userID 的所有未读消息 ID（已读回执推送用）。
func (r *GormMessageRepository) FindUnreadMessageIDs(userID uint, peerID uint) ([]uint, error) {
	var ids []uint
	if err := r.db.Model(&model.Message{}).
		Where("from_uid = ? AND to_uid = ? AND is_read = false", peerID, userID).
		Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

// DeleteMessage 删除消息（软删除）
func (r *GormMessageRepository) DeleteMessage(message *model.Message) error {
	return r.db.Delete(message).Error
}

// 用户相关方法
func (r *GormMessageRepository) FindUserByID(userID uint) (*model.User, error) {
	var user model.User
	if err := r.db.First(&user, userID).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// Conversation 会话列表结构
type Conversation struct {
	PeerUID  uint   `json:"peer_uid"`
	Username string `json:"username"`
	LastMsg  string `json:"last_msg"`
	LastTime string `json:"last_time"`
	Unread   int64  `json:"unread"`
}
