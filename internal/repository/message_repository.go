package repository

import (
	"go-admin/model"

	"gorm.io/gorm"
)

// MessageRepository 定义消息相关的数据库操作接口
type MessageRepository interface {
	// 消息相关
	CreateMessage(message *model.Message) error
	FindMessageByID(id uint) (*model.Message, error)
	GetConversations(userID uint) ([]Conversation, error)
	GetMessages(userID uint, peerID uint, limit int) ([]model.Message, error)
	MarkMessagesAsRead(userID uint, peerID uint) error
	CountUnreadMessages(userID uint, peerID uint) (int64, error)
	DeleteMessage(message *model.Message) error

	// 用户相关
	FindUserByID(userID uint) (*model.User, error)
}

// GormMessageRepository 基于GORM的消息Repository实现
type GormMessageRepository struct {
	db *gorm.DB
}

// NewGormMessageRepository 构造函数：注入DB依赖
func NewGormMessageRepository(db *gorm.DB) *GormMessageRepository {
	return &GormMessageRepository{db: db}
}

// 消息相关方法
func (r *GormMessageRepository) CreateMessage(message *model.Message) error {
	return r.db.Create(message).Error
}

func (r *GormMessageRepository) FindMessageByID(id uint) (*model.Message, error) {
	var message model.Message
	if err := r.db.First(&message, id).Error; err != nil {
		return nil, err
	}
	return &message, nil
}

func (r *GormMessageRepository) GetConversations(userID uint) ([]Conversation, error) {
	var conversations []Conversation

	// 使用原生SQL查询会话列表
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

func (r *GormMessageRepository) GetMessages(userID uint, peerID uint, limit int) ([]model.Message, error) {
	var messages []model.Message

	if err := r.db.Where("(from_uid = ? AND to_uid = ?) OR (from_uid = ? AND to_uid = ?)",
		userID, peerID, peerID, userID).
		Order("created_at DESC").
		Limit(limit).
		Find(&messages).Error; err != nil {
		return nil, err
	}

	return messages, nil
}

func (r *GormMessageRepository) MarkMessagesAsRead(userID uint, peerID uint) error {
	return r.db.Model(&model.Message{}).
		Where("from_uid = ? AND to_uid = ? AND is_read = false", peerID, userID).
		Update("is_read", true).Error
}

func (r *GormMessageRepository) CountUnreadMessages(userID uint, peerID uint) (int64, error) {
	var count int64

	if err := r.db.Model(&model.Message{}).
		Where("from_uid = ? AND to_uid = ? AND is_read = false", peerID, userID).
		Count(&count).Error; err != nil {
		return 0, err
	}

	return count, nil
}

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

// Conversation 会话结构
type Conversation struct {
	PeerUID  uint   `json:"peer_uid"`
	Username string `json:"username"`
	LastMsg  string `json:"last_msg"`
	LastTime string `json:"last_time"`
	Unread   int64  `json:"unread"`
}
