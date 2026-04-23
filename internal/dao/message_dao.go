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
	return r.db.Model(&model.Message{}).
		Where("from_uid = ? AND to_uid = ? AND is_read = false", peerID, userID).
		Update("is_read", true).Error
}

// CountUnreadMessages 统计未读消息数量
func (r *GormMessageRepository) CountUnreadMessages(userID uint, peerID uint) (int64, error) {
	var count int64

	if err := r.db.Model(&model.Message{}).
		Where("from_uid = ? AND to_uid = ? AND is_read = false", peerID, userID).
		Count(&count).Error; err != nil {
		return 0, err
	}

	return count, nil
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
