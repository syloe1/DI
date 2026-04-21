package dao

import (
	"go-admin/internal/domain/model"

	"gorm.io/gorm"
)

// CommentRepository 瀹氫箟璇勮鐩稿叧鐨勬暟鎹簱鎿嶄綔鎺ュ彛
type CommentRepository interface {
	Create(comment *model.Comment) error
	FindByID(id string) (*model.Comment, error)
	FindByPostID(postID string) ([]model.Comment, error)
	FindByUserID(userID uint) ([]model.Comment, error)
	Update(comment *model.Comment, updates map[string]interface{}) error
	Delete(comment *model.Comment) error
	FindUserByID(userID uint) (*model.User, error)
	FindPostByID(postID uint) (*model.Post, error)
}

// GormCommentRepository 鍩轰簬GORM鐨勮瘎璁篟epository瀹炵幇
type GormCommentRepository struct {
	db *gorm.DB
}

// NewGormCommentRepository 鏋勯€犲嚱鏁帮細娉ㄥ叆DB渚濊禆
func NewGormCommentRepository(db *gorm.DB) *GormCommentRepository {
	return &GormCommentRepository{db: db}
}

func (r *GormCommentRepository) Create(comment *model.Comment) error {
	return r.db.Create(comment).Error
}

func (r *GormCommentRepository) FindByID(id string) (*model.Comment, error) {
	var comment model.Comment
	if err := r.db.First(&comment, id).Error; err != nil {
		return nil, err
	}
	return &comment, nil
}

func (r *GormCommentRepository) FindByPostID(postID string) ([]model.Comment, error) {
	var comments []model.Comment
	if err := r.db.Where("post_id = ? AND status = ?", postID, model.CommentStatusNormal).
		Preload("User").
		Order("created_at asc").Find(&comments).Error; err != nil {
		return nil, err
	}
	return comments, nil
}

func (r *GormCommentRepository) FindByUserID(userID uint) ([]model.Comment, error) {
	var comments []model.Comment
	if err := r.db.Where("user_id = ?", userID).Find(&comments).Error; err != nil {
		return nil, err
	}
	return comments, nil
}

func (r *GormCommentRepository) Update(comment *model.Comment, updates map[string]interface{}) error {
	return r.db.Model(comment).Updates(updates).Error
}

func (r *GormCommentRepository) Delete(comment *model.Comment) error {
	return r.db.Delete(comment).Error
}

func (r *GormCommentRepository) FindUserByID(userID uint) (*model.User, error) {
	var user model.User
	if err := r.db.First(&user, userID).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *GormCommentRepository) FindPostByID(postID uint) (*model.Post, error) {
	var post model.Post
	if err := r.db.First(&post, postID).Error; err != nil {
		return nil, err
	}
	return &post, nil
}
