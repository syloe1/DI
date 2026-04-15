package repository

import (
	"go-admin/model"

	"gorm.io/gorm"
)

// PostRepository 定义帖子相关的数据库操作接口
type PostRepository interface {
	Create(post *model.Post) error
	FindByID(id string) (*model.Post, error)
	FindPublicByTopic(topic string) ([]model.Post, error)
	FindByUserID(userID uint) ([]model.Post, error)
	FindPublicByUserID(userID string) ([]model.Post, error)
	FindLikedByUserID(userID string) ([]model.Post, error)
	FindCollectedByUserID(userID string) ([]model.Post, error)
	Update(post *model.Post, updates map[string]interface{}) error
	Delete(post *model.Post) error
}

// GormPostRepository 基于GORM的实现（适配原逻辑）
type GormPostRepository struct {
	db *gorm.DB // 注入的DB实例
}

// NewGormPostRepository 构造函数：注入DB依赖
func NewGormPostRepository(db *gorm.DB) *GormPostRepository {
	return &GormPostRepository{db: db}
}

// 实现PostRepository接口的所有方法（适配原API层的DB操作）
func (r *GormPostRepository) Create(post *model.Post) error {
	return r.db.Create(post).Error
}

func (r *GormPostRepository) FindByID(id string) (*model.Post, error) {
	var post model.Post
	if err := r.db.First(&post, id).Error; err != nil {
		return nil, err
	}
	return &post, nil
}

func (r *GormPostRepository) FindPublicByTopic(topic string) ([]model.Post, error) {
	var posts []model.Post
	query := r.db.Where("status = ? AND is_public = true", model.PostStatusPublished)
	if topic != "" {
		query = query.Where("topics LIKE ?", "%"+topic+"%") // 注意原代码笔误：topic→topics
	}
	err := query.Order("created_at desc").Find(&posts).Error
	return posts, err
}

func (r *GormPostRepository) FindByUserID(userID uint) ([]model.Post, error) {
	var posts []model.Post
	err := r.db.Where("user_id = ?", userID).Find(&posts).Error
	return posts, err
}

func (r *GormPostRepository) FindPublicByUserID(userID string) ([]model.Post, error) {
	var posts []model.Post
	err := r.db.Where("user_id = ? AND is_public = true AND status = ?", userID, model.PostStatusPublished).
		Order("created_at desc").Find(&posts).Error
	return posts, err
}

func (r *GormPostRepository) FindLikedByUserID(userID string) ([]model.Post, error) {
	var posts []model.Post
	err := r.db.Table("posts").
		Joins("JOIN likes ON posts.id = likes.post_id").
		Where("likes.user_id = ? AND posts.is_public = true AND posts.status = ?", userID, model.PostStatusPublished).
		Order("likes.created_at desc").Find(&posts).Error
	return posts, err
}

func (r *GormPostRepository) FindCollectedByUserID(userID string) ([]model.Post, error) {
	var posts []model.Post
	err := r.db.Table("posts").
		Joins("JOIN collects ON posts.id = collects.post_id").
		Where("collects.user_id = ? AND posts.is_public = true AND posts.status = ?", userID, model.PostStatusPublished).
		Order("collects.created_at desc").Find(&posts).Error
	return posts, err
}

func (r *GormPostRepository) Update(post *model.Post, updates map[string]interface{}) error {
	return r.db.Model(post).Updates(updates).Error
}

func (r *GormPostRepository) Delete(post *model.Post) error {
	return r.db.Delete(post).Error
}
