package dao

import (
	"go-admin/internal/domain/model"

	"gorm.io/gorm"
)

// GormUserDB 基于GORM实现的用户数据访问对象
// 实现了 UserDB 接口，负责与数据库进行交互
type GormUserDB struct {
	DB *gorm.DB // 数据库连接对象
}

// Create 创建用户
// 将用户信息插入到数据库中
func (g *GormUserDB) Create(user *model.User) error {
	return g.DB.Create(user).Error
}

// FindByID 根据用户ID查询用户信息
// 通过主键ID获取单条用户记录
func (g *GormUserDB) FindByID(id uint) (*model.User, error) {
	var user model.User
	if err := g.DB.First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByUsername 根据用户名查询用户
// 用于登录、用户名重复校验等场景
func (g *GormUserDB) FindByUsername(username string) (*model.User, error) {
	var user model.User
	if err := g.DB.Where("username = ?", username).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// FindAll 查询所有用户
// 获取用户列表，常用于后台管理展示全部用户
func (g *GormUserDB) FindAll() ([]model.User, error) {
	var users []model.User
	if err := g.DB.Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// CountByUsername 统计指定用户名的数量
// 用于校验用户名是否已被注册
func (g *GormUserDB) CountByUsername(username string) (int64, error) {
	var count int64
	if err := g.DB.Model(&model.User{}).Where("username = ?", username).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// FindByUsernameLike 用户名模糊查询
// 根据输入的关键词模糊匹配用户名，用于用户搜索功能
func (g *GormUserDB) FindByUsernameLike(username string, limit int) ([]model.User, error) {
	var users []model.User
	// 注意：外部调用时需要拼接 % 关键字% 才能实现模糊搜索
	if err := g.DB.Where("username LIKE ?", username).Limit(limit).Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// FindByIDs 根据用户ID列表批量查询用户
// 一次性查询多个用户，提高接口效率
func (g *GormUserDB) FindByIDs(ids []uint) ([]model.User, error) {
	var users []model.User
	if err := g.DB.Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// Update 更新用户信息
// 保存用户的修改数据到数据库
func (g *GormUserDB) Update(user *model.User) error {
	return g.DB.Save(user).Error
}

// Delete 根据ID删除用户
// 根据主键ID删除对应的用户记录
func (g *GormUserDB) Delete(id uint) error {
	return g.DB.Delete(&model.User{}, id).Error
}
