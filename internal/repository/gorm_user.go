package repository

import (
	"go-admin/model"
	"gorm.io/gorm"
)

// GormUserDB 基于GORM的UserDB实现
type GormUserDB struct {
	DB *gorm.DB
}

func (g *GormUserDB) Create(user *model.User) error {
	return g.DB.Create(user).Error
}

func (g *GormUserDB) FindByID(id uint) (*model.User, error) {
	var user model.User
	if err := g.DB.First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (g *GormUserDB) FindByUsername(username string) (*model.User, error) {
	var user model.User
	if err := g.DB.Where("username = ?", username).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (g *GormUserDB) FindAll() ([]model.User, error) {
	var users []model.User
	if err := g.DB.Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func (g *GormUserDB) CountByUsername(username string) (int64, error) {
	var count int64
	if err := g.DB.Model(&model.User{}).Where("username = ?", username).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (g *GormUserDB) FindByUsernameLike(username string, limit int) ([]model.User, error) {
	var users []model.User
	if err := g.DB.Where("username LIKE ?", username).Limit(limit).Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func (g *GormUserDB) FindByIDs(ids []uint) ([]model.User, error) {
	var users []model.User
	if err := g.DB.Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func (g *GormUserDB) Update(user *model.User) error {
	return g.DB.Save(user).Error
}

func (g *GormUserDB) Delete(id uint) error {
	return g.DB.Delete(&model.User{}, id).Error
}