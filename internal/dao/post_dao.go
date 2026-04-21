package dao

import (
	"go-admin/internal/domain/model"

	"gorm.io/gorm"
)

type PostRepository interface {
	Create(post *model.Post) error
	FindByID(id string) (*model.Post, error)
	FindByIDs(ids []uint) ([]model.Post, error)
	FindPublicByTopic(topic string) ([]model.Post, error)
	FindPublicByTopicWithStrategy(topic string, strategy PostSortStrategy) ([]model.Post, error)
	FindPublicPage(topic string, page int, pageSize int, strategy PostSortStrategy) ([]model.Post, int64, error)
	FindByUserID(userID uint) ([]model.Post, error)
	FindPublicByUserID(userID string) ([]model.Post, error)
	FindLikedByUserID(userID string) ([]model.Post, error)
	FindCollectedByUserID(userID string) ([]model.Post, error)
	Update(post *model.Post, updates map[string]interface{}) error
	Delete(post *model.Post) error
}

type GormPostRepository struct {
	db *gorm.DB
}

func NewGormPostRepository(db *gorm.DB) *GormPostRepository {
	return &GormPostRepository{db: db}
}

func (r *GormPostRepository) Create(post *model.Post) error {
	return r.db.Create(post).Error
}

func (r *GormPostRepository) FindByID(id string) (*model.Post, error) {
	var post model.Post
	if err := r.db.Preload("User").First(&post, id).Error; err != nil {
		return nil, err
	}
	return &post, nil
}

func (r *GormPostRepository) FindByIDs(ids []uint) ([]model.Post, error) {
	var posts []model.Post
	if len(ids) == 0 {
		return posts, nil
	}
	err := r.db.Preload("User").Where("id IN ?", ids).Find(&posts).Error
	return posts, err
}

func (r *GormPostRepository) FindPublicByTopic(topic string) ([]model.Post, error) {
	return r.FindPublicByTopicWithStrategy(topic, TimeSortStrategy{})
}

func (r *GormPostRepository) FindPublicByTopicWithStrategy(topic string, strategy PostSortStrategy) ([]model.Post, error) {
	var posts []model.Post
	query := r.db.Scopes(WithPublishedPost(), WithTopic(topic))
	if strategy == nil {
		strategy = TimeSortStrategy{}
	}
	err := strategy.Apply(query).Find(&posts).Error
	return posts, err
}

func (r *GormPostRepository) FindPublicPage(topic string, page int, pageSize int, strategy PostSortStrategy) ([]model.Post, int64, error) {
	var (
		posts []model.Post
		total int64
	)

	baseQuery := r.db.Model(&model.Post{}).Scopes(WithPublishedPost(), WithTopic(topic))
	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query := r.db.Scopes(WithPublishedPost(), WithTopic(topic), WithPagination(page, pageSize))
	if strategy == nil {
		strategy = TimeSortStrategy{}
	}
	if err := strategy.Apply(query).Find(&posts).Error; err != nil {
		return nil, 0, err
	}

	return posts, total, nil
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
