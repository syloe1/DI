package dto

type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=32,username_format"`
	Password string `json:"password" binding:"required,min=6,max=72"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"required,min=3,max=32,username_format"`
	Password string `json:"password" binding:"required,min=6,max=72"`
}

type AddUserRequest struct {
	Username string `json:"username" binding:"required,min=3,max=32,username_format"`
	Password string `json:"password" binding:"required,min=6,max=72"`
	Role     string `json:"role" binding:"omitempty,oneof=user admin superadmin"`
}

type UpdateUserRequest struct {
	Username string `json:"username" binding:"omitempty,min=3,max=32,username_format"`
	Role     string `json:"role" binding:"omitempty,oneof=user admin superadmin"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword" binding:"omitempty,min=6,max=72"`
	NewPassword string `json:"newPassword" binding:"required,min=6,max=72"`
}

type BatchGetUserRolesRequest struct {
	IDs []uint `json:"ids" binding:"required,min=1,max=100,dive,min=1"`
}
