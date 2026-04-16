package repository

import (
	"go-admin/model"
	"gorm.io/gorm"
)

// SocialRepository 定义社交相关的数据库操作接口
type SocialRepository interface {
	WithTx(tx *gorm.DB) SocialRepository
	Transaction(fn func(repo SocialRepository) error) error

	// 关注相关
	FindFollowRelation(fromUID uint, toUID uint) (*model.UserRelation, error)
	CreateFollowRelation(relation *model.UserRelation) error
	DeleteFollowRelation(relation *model.UserRelation) error
	CountFollowers(userID uint) (int64, error)
	CountFollowing(userID uint) (int64, error)
	GetFollowers(userID uint) ([]model.UserRelation, error)
	GetFollowing(userID uint) ([]model.UserRelation, error)
	
	// 拉黑相关
	FindBlockRelation(fromUID uint, toUID uint) (*model.UserRelation, error)
	CreateBlockRelation(relation *model.UserRelation) error
	DeleteBlockRelation(relation *model.UserRelation) error
	GetBlockedUsers(userID uint) ([]model.UserRelation, error)
	
	// 关系状态查询
	GetRelationStatus(fromUID uint, toUID uint) (map[string]interface{}, error)
	
	// 用户查询
	FindUserByID(userID uint) (*model.User, error)
}

// GormSocialRepository 基于GORM的社交Repository实现
type GormSocialRepository struct {
	db *gorm.DB
}

// NewGormSocialRepository 构造函数：注入DB依赖
func NewGormSocialRepository(db *gorm.DB) *GormSocialRepository {
	return &GormSocialRepository{db: db}
}

func (r *GormSocialRepository) WithTx(tx *gorm.DB) SocialRepository {
	return &GormSocialRepository{db: tx}
}

func (r *GormSocialRepository) Transaction(fn func(repo SocialRepository) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return fn(r.WithTx(tx))
	})
}

// 关注相关方法
func (r *GormSocialRepository) FindFollowRelation(fromUID uint, toUID uint) (*model.UserRelation, error) {
	var relation model.UserRelation
	if err := r.db.Where("from_uid = ? AND to_uid = ? AND type = 'follow'", fromUID, toUID).
		First(&relation).Error; err != nil {
		return nil, err
	}
	return &relation, nil
}

func (r *GormSocialRepository) CreateFollowRelation(relation *model.UserRelation) error {
	return r.db.Create(relation).Error
}

func (r *GormSocialRepository) DeleteFollowRelation(relation *model.UserRelation) error {
	return r.db.Delete(relation).Error
}

func (r *GormSocialRepository) CountFollowers(userID uint) (int64, error) {
	var count int64
	if err := r.db.Model(&model.UserRelation{}).
		Where("to_uid = ? AND type = 'follow'", userID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *GormSocialRepository) CountFollowing(userID uint) (int64, error) {
	var count int64
	if err := r.db.Model(&model.UserRelation{}).
		Where("from_uid = ? AND type = 'follow'", userID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *GormSocialRepository) GetFollowers(userID uint) ([]model.UserRelation, error) {
	var relations []model.UserRelation
	if err := r.db.Where("to_uid = ? AND type = 'follow'", userID).
		Find(&relations).Error; err != nil {
		return nil, err
	}
	return relations, nil
}

func (r *GormSocialRepository) GetFollowing(userID uint) ([]model.UserRelation, error) {
	var relations []model.UserRelation
	if err := r.db.Where("from_uid = ? AND type = 'follow'", userID).
		Find(&relations).Error; err != nil {
		return nil, err
	}
	return relations, nil
}

// 拉黑相关方法
func (r *GormSocialRepository) FindBlockRelation(fromUID uint, toUID uint) (*model.UserRelation, error) {
	var relation model.UserRelation
	if err := r.db.Where("from_uid = ? AND to_uid = ? AND type = 'block'", fromUID, toUID).
		First(&relation).Error; err != nil {
		return nil, err
	}
	return &relation, nil
}

func (r *GormSocialRepository) CreateBlockRelation(relation *model.UserRelation) error {
	return r.db.Create(relation).Error
}

func (r *GormSocialRepository) DeleteBlockRelation(relation *model.UserRelation) error {
	return r.db.Delete(relation).Error
}

func (r *GormSocialRepository) GetBlockedUsers(userID uint) ([]model.UserRelation, error) {
	var relations []model.UserRelation
	if err := r.db.Where("from_uid = ? AND type = 'block'", userID).
		Find(&relations).Error; err != nil {
		return nil, err
	}
	return relations, nil
}

// 关系状态查询
func (r *GormSocialRepository) GetRelationStatus(fromUID uint, toUID uint) (map[string]interface{}, error) {
	status := make(map[string]interface{})
	
	// 检查关注状态
	follow, _ := r.FindFollowRelation(fromUID, toUID)
	status["following"] = follow != nil
	
	// 检查被关注状态
	followed, _ := r.FindFollowRelation(toUID, fromUID)
	status["followed"] = followed != nil
	
	// 检查拉黑状态
	block, _ := r.FindBlockRelation(fromUID, toUID)
	status["blocked"] = block != nil
	
	// 检查被拉黑状态
	blocked, _ := r.FindBlockRelation(toUID, fromUID)
	status["blocked_by"] = blocked != nil
	
	return status, nil
}

// 用户查询
func (r *GormSocialRepository) FindUserByID(userID uint) (*model.User, error) {
	var user model.User
	if err := r.db.First(&user, userID).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
