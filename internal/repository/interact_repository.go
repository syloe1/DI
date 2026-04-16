package repository

import (
	"go-admin/model"

	"gorm.io/gorm"
)

// InteractRepository 定义互动相关的数据库操作接口
type InteractRepository interface {
	WithTx(tx *gorm.DB) InteractRepository
	Transaction(fn func(repo InteractRepository) error) error

	// 点赞相关
	FindLike(userID uint, postID uint) (*model.Like, error)
	CreateLike(like *model.Like) error
	DeleteLike(like *model.Like) error
	FindDislike(userID uint, postID uint) (*model.Dislike, error)
	CreateDislike(dislike *model.Dislike) error
	DeleteDislike(dislike *model.Dislike) error
	CountLikes(postID uint) (int64, error)
	CountDislikes(postID uint) (int64, error)

	// 收藏相关
	FindCollect(userID uint, postID uint) (*model.Collect, error)
	CreateCollect(collect *model.Collect) error
	DeleteCollect(collect *model.Collect) error
	CountCollects(postID uint) (int64, error)

	// 分享相关
	CreateShare(share *model.Share) error
	CountShares(postID uint) (int64, error)

	// 互动状态查询
	GetInteractStatus(userID uint, postID uint) (map[string]interface{}, error)
	GetInteractCount(postID uint) (map[string]int64, error)
}

// GormInteractRepository 基于GORM的互动Repository实现
type GormInteractRepository struct {
	db *gorm.DB
}

// NewGormInteractRepository 构造函数：注入DB依赖
func NewGormInteractRepository(db *gorm.DB) *GormInteractRepository {
	return &GormInteractRepository{db: db}
}

func (r *GormInteractRepository) WithTx(tx *gorm.DB) InteractRepository {
	return &GormInteractRepository{db: tx}
}

func (r *GormInteractRepository) Transaction(fn func(repo InteractRepository) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return fn(r.WithTx(tx))
	})
}

// 点赞相关方法
func (r *GormInteractRepository) FindLike(userID uint, postID uint) (*model.Like, error) {
	var like model.Like
	if err := r.db.Where("user_id = ? AND post_id = ?", userID, postID).First(&like).Error; err != nil {
		return nil, err
	}
	return &like, nil
}

func (r *GormInteractRepository) CreateLike(like *model.Like) error {
	return r.db.Create(like).Error
}

func (r *GormInteractRepository) DeleteLike(like *model.Like) error {
	return r.db.Delete(like).Error
}

func (r *GormInteractRepository) FindDislike(userID uint, postID uint) (*model.Dislike, error) {
	var dislike model.Dislike
	if err := r.db.Where("user_id = ? AND post_id = ?", userID, postID).First(&dislike).Error; err != nil {
		return nil, err
	}
	return &dislike, nil
}

func (r *GormInteractRepository) CreateDislike(dislike *model.Dislike) error {
	return r.db.Create(dislike).Error
}

func (r *GormInteractRepository) DeleteDislike(dislike *model.Dislike) error {
	return r.db.Delete(dislike).Error
}

func (r *GormInteractRepository) CountLikes(postID uint) (int64, error) {
	var count int64
	if err := r.db.Model(&model.Like{}).Where("post_id = ?", postID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *GormInteractRepository) CountDislikes(postID uint) (int64, error) {
	var count int64
	if err := r.db.Model(&model.Dislike{}).Where("post_id = ?", postID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// 收藏相关方法
func (r *GormInteractRepository) FindCollect(userID uint, postID uint) (*model.Collect, error) {
	var collect model.Collect
	if err := r.db.Where("user_id = ? AND post_id = ?", userID, postID).First(&collect).Error; err != nil {
		return nil, err
	}
	return &collect, nil
}

func (r *GormInteractRepository) CreateCollect(collect *model.Collect) error {
	return r.db.Create(collect).Error
}

func (r *GormInteractRepository) DeleteCollect(collect *model.Collect) error {
	return r.db.Delete(collect).Error
}

func (r *GormInteractRepository) CountCollects(postID uint) (int64, error) {
	var count int64
	if err := r.db.Model(&model.Collect{}).Where("post_id = ?", postID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// 分享相关方法
func (r *GormInteractRepository) CreateShare(share *model.Share) error {
	return r.db.Create(share).Error
}

func (r *GormInteractRepository) CountShares(postID uint) (int64, error) {
	var count int64
	if err := r.db.Model(&model.Share{}).Where("post_id = ?", postID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// 互动状态查询
func (r *GormInteractRepository) GetInteractStatus(userID uint, postID uint) (map[string]interface{}, error) {
	status := make(map[string]interface{})

	// 检查点赞状态
	like, _ := r.FindLike(userID, postID)
	status["liked"] = like != nil

	// 检查点踩状态
	dislike, _ := r.FindDislike(userID, postID)
	status["disliked"] = dislike != nil

	// 检查收藏状态
	collect, _ := r.FindCollect(userID, postID)
	status["collected"] = collect != nil

	return status, nil
}

func (r *GormInteractRepository) GetInteractCount(postID uint) (map[string]int64, error) {
	counts := make(map[string]int64)

	likes, _ := r.CountLikes(postID)
	counts["likes"] = likes

	dislikes, _ := r.CountDislikes(postID)
	counts["dislikes"] = dislikes

	collects, _ := r.CountCollects(postID)
	counts["collects"] = collects

	shares, _ := r.CountShares(postID)
	counts["shares"] = shares

	return counts, nil
}
