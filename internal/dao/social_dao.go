package dao

import (
	"go-admin/internal/domain/model"

	"gorm.io/gorm"
)

type SocialRepository interface {
	WithTx(tx *gorm.DB) SocialRepository
	Transaction(fn func(repo SocialRepository) error) error

	FindFollowRelation(fromUID uint, toUID uint) (*model.UserRelation, error)
	CreateFollowRelation(relation *model.UserRelation) error
	DeleteFollowRelation(relation *model.UserRelation) error
	CountFollowers(userID uint) (int64, error)
	CountFollowing(userID uint) (int64, error)
	GetFollowers(userID uint) ([]model.UserRelation, error)
	GetFollowing(userID uint) ([]model.UserRelation, error)

	FindBlockRelation(fromUID uint, toUID uint) (*model.UserRelation, error)
	CreateBlockRelation(relation *model.UserRelation) error
	DeleteBlockRelation(relation *model.UserRelation) error
	GetBlockedUsers(userID uint) ([]model.UserRelation, error)

	GetRelationStatus(fromUID uint, toUID uint) (map[string]interface{}, error)
	FindUserByID(userID uint) (*model.User, error)
}

type GormSocialRepository struct {
	db *gorm.DB
}

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

func (r *GormSocialRepository) GetRelationStatus(fromUID uint, toUID uint) (map[string]interface{}, error) {
	status := make(map[string]interface{})

	follow, _ := r.FindFollowRelation(fromUID, toUID)
	status["following"] = follow != nil

	followed, _ := r.FindFollowRelation(toUID, fromUID)
	status["followed"] = followed != nil

	block, _ := r.FindBlockRelation(fromUID, toUID)
	status["blocked"] = block != nil

	blocked, _ := r.FindBlockRelation(toUID, fromUID)
	status["blocked_by"] = blocked != nil

	return status, nil
}

func (r *GormSocialRepository) FindUserByID(userID uint) (*model.User, error) {
	var user model.User
	if err := r.db.First(&user, userID).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
