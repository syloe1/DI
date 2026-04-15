package model

import (
	"gorm.io/gorm"
)

const (
	RoleSuperAdmin = "superadmin" //最高权限
	RoleAdmin      = "admin"      //管理内容
	RoleUser       = "user"       //只能操作自己
)

type User struct {
	gorm.Model
	Username string `gorm:"size:32;not null;unique" json:"username"`
	Password string `gorm:"size:64;not null; json:"-"` //密码哈希不给前端
	Role     string `gorm:"size:16;default:user" json:"role"`
}
