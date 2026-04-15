package api

import (
	"context"
	"encoding/json"
	"fmt"
	"go-admin/core"
	"go-admin/internal/repository"
	"go-admin/model"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/bcrypt"
)

type UserService struct {
	db     repository.UserDB
	cache  repository.UserCache
	jwtCfg repository.JWTConfig
	jwtKey []byte
	ctx    context.Context
}

// NewUserService 构造函数，依赖注入
func NewUserService(db repository.UserDB, cache repository.UserCache, jwtCfg repository.JWTConfig, jwtKey []byte, ctx context.Context) *UserService {
	return &UserService{
		db:     db,
		cache:  cache,
		jwtCfg: jwtCfg,
		jwtKey: jwtKey,
		ctx:    ctx,
	}
}

type Claims struct { //JWT载荷
	UserId   uint   `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// GetJwtKey 获取JWT密钥（适配原有逻辑）
func (s *UserService) GetJwtKey() []byte {
	if len(s.jwtKey) > 0 {
		return s.jwtKey
	}
	return s.jwtCfg.GetSecret()
}

func (s *UserService) AddUser(c *gin.Context) {
	role := c.GetString("role")
	if role != "admin" && role != "superadmin" {
		core.Fail(c, http.StatusForbidden, "无权限添加用户")
		return
	}

	var user model.User
	if err := c.ShouldBindJSON(&user); err != nil {
		core.Fail(c, http.StatusBadRequest, "参数错误")
		return
	}

	// 使用依赖注入的数据库操作
	count, err := s.db.CountByUsername(user.Username)
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "查询用户名失败")
		return
	}

	if count > 0 {
		core.Fail(c, http.StatusBadRequest, "用户名已存在")
		return
	}

	if user.Role == "" {
		user.Role = "user"
	}

	// 密码加密
	if user.Password == "" {
		core.Fail(c, http.StatusBadRequest, "密码不能为空")
		return
	}

	hashPasswd, err := bcrypt.GenerateFromPassword([]byte(user.Password), 10)
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "密码加密失败")
		return
	}
	user.Password = string(hashPasswd)

	// 使用依赖注入的数据库创建操作
	if err := s.db.Create(&user); err != nil {
		core.Fail(c, http.StatusInternalServerError, "创建用户失败")
		return
	}

	// 使用依赖注入的缓存操作
	s.cache.Del(s.ctx, "user:list")
	core.Success(c, "添加成功")
}

func (s *UserService) GetUserList(c *gin.Context) {
	role := c.GetString("role")
	if role != "admin" && role != "superadmin" {
		core.Fail(c, http.StatusForbidden, "无权限查看用户列表")
		return
	}

	key := "user:list"

	// 使用依赖注入的缓存操作
	val, err := s.cache.Get(s.ctx, key)
	if err == nil {
		var safeList []gin.H
		_ = json.Unmarshal([]byte(val), &safeList)
		core.Success(c, safeList)
		return
	}

	// 使用依赖注入的数据库查询操作
	userList, err := s.db.FindAll()
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "查询用户列表失败")
		return
	}

	safeList := make([]gin.H, len(userList))
	for i, u := range userList {
		safeList[i] = gin.H{
			"ID":        u.ID,
			"Username":  u.Username,
			"Role":      u.Role,
			"CreatedAt": u.CreatedAt,
		}
	}

	// 写入缓存
	listJson, _ := json.Marshal(safeList)
	s.cache.Set(s.ctx, key, string(listJson), 5*time.Minute)

	core.Success(c, safeList)
}

// GetUser 根据ID查询单个用户
func (s *UserService) GetUser(c *gin.Context) {
	idStr := c.Param("id")

	// 转换ID为uint
	var id uint
	_, err := fmt.Sscanf(idStr, "%d", &id)
	if err != nil {
		core.Fail(c, http.StatusBadRequest, "用户ID格式错误")
		return
	}

	key := "user:" + idStr

	// 读取缓存
	val, err := s.cache.Get(s.ctx, key)
	if err == nil {
		var user model.User
		json.Unmarshal([]byte(val), &user)
		core.Success(c, user)
		return
	}

	// 读取数据库
	user, err := s.db.FindByID(id)
	if err != nil {
		core.Fail(c, http.StatusNotFound, "用户不存在")
		return
	}

	// 写入缓存
	userJson, _ := json.Marshal(user)
	s.cache.Set(s.ctx, key, string(userJson), 5*time.Minute)

	core.Success(c, user)
}

// UpdateUser 修改用户信息（改造为依赖注入）
func (s *UserService) UpdateUser(c *gin.Context) {
	idStr := c.Param("id")
	currentUserID := c.GetUint("user_id")
	currentRole := c.GetString("role")

	// 转换ID为uint
	var id uint
	_, err := fmt.Sscanf(idStr, "%d", &id)
	if err != nil {
		core.Fail(c, http.StatusBadRequest, "用户ID格式错误")
		return
	}

	// 使用依赖注入的数据库操作查询用户
	user, err := s.db.FindByID(id)
	if err != nil {
		core.Fail(c, http.StatusNotFound, "用户不存在")
		return
	}

	// 权限：只能改自己 或 管理员改任何人
	if user.ID != currentUserID && currentRole != "admin" && currentRole != "superadmin" {
		core.Fail(c, http.StatusForbidden, "无权限修改他人信息")
		return
	}

	var req model.User
	if err := c.ShouldBindJSON(&req); err != nil {
		core.Fail(c, http.StatusBadRequest, "参数错误")
		return
	}

	if req.Username != "" {
		user.Username = req.Username
	}
	if req.Role != "" {
		if currentRole == "superadmin" {
			user.Role = req.Role
		} else if currentRole == "admin" && req.Role == "user" {
			user.Role = req.Role
		}
	}

	// 使用依赖注入的数据库更新操作
	if err := s.db.Update(user); err != nil {
		core.Fail(c, http.StatusInternalServerError, "修改用户信息失败")
		return
	}

	// 清除缓存（依赖注入的缓存操作）
	s.cache.Del(s.ctx, "user:"+idStr)
	s.cache.Del(s.ctx, "user:list")
	core.Success(c, "修改成功")
}

// ChangePassword 修改密码（改造为依赖注入）
func (s *UserService) ChangePassword(c *gin.Context) {
	idStr := c.Param("id")
	currentUserID := c.GetUint("userID")
	currentRole := c.GetString("role")

	// 转换ID为uint
	var id uint
	_, err := fmt.Sscanf(idStr, "%d", &id)
	if err != nil {
		core.Fail(c, http.StatusBadRequest, "用户ID格式错误")
		return
	}

	// 使用依赖注入的数据库操作查询用户
	user, err := s.db.FindByID(id)
	if err != nil {
		core.Fail(c, http.StatusNotFound, "用户不存在")
		return
	}

	// 权限检查：
	// 1. 超级管理员可以修改任何用户的密码
	// 2. 管理员可以修改自己和普通用户的密码
	// 3. 普通用户只能修改自己的密码
	if id != currentUserID {
		if currentRole == "superadmin" {
			// 超级管理员可以修改任何人的密码
		} else if currentRole == "admin" {
			// 管理员只能修改普通用户的密码
			if user.Role == "admin" || user.Role == "superadmin" {
				core.Fail(c, http.StatusForbidden, "管理员只能修改普通用户的密码")
				return
			}
		} else {
			// 普通用户只能修改自己的密码
			core.Fail(c, http.StatusForbidden, "只能修改自己的密码")
			return
		}
	}

	var req struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		core.Fail(c, http.StatusBadRequest, "参数错误")
		return
	}

	// 验证原密码（只有非管理员修改自己密码时需要）
	if currentRole != "admin" && currentRole != "superadmin" {
		err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.OldPassword))
		if err != nil {
			core.Fail(c, http.StatusBadRequest, "原密码错误")
			return
		}
	}

	// 加密新密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "密码加密失败")
		return
	}

	// 更新密码
	user.Password = string(hashedPassword)
	if err := s.db.Update(user); err != nil {
		core.Fail(c, http.StatusInternalServerError, "修改密码失败")
		return
	}

	// 清除缓存（依赖注入的缓存操作）
	s.cache.Del(s.ctx, "user:"+idStr)
	s.cache.Del(s.ctx, "user:list")

	core.Success(c, "密码修改成功")
}

// DeleteUser 删除用户（改造为依赖注入）
func (s *UserService) DeleteUser(c *gin.Context) {
	role := c.GetString("role")
	if role != "admin" && role != "superadmin" {
		core.Fail(c, http.StatusForbidden, "无权限删除")
		return
	}

	idStr := c.Param("id")
	var id uint
	_, err := fmt.Sscanf(idStr, "%d", &id)
	if err != nil {
		core.Fail(c, http.StatusBadRequest, "用户ID格式错误")
		return
	}

	// 检查用户是否存在
	_, err = s.db.FindByID(id)
	if err != nil {
		core.Fail(c, http.StatusNotFound, "用户不存在")
		return
	}

	// 使用依赖注入的数据库删除操作
	if err := s.db.Delete(id); err != nil {
		core.Fail(c, http.StatusInternalServerError, "删除用户失败")
		return
	}

	// 清除缓存（依赖注入的缓存操作）
	s.cache.Del(s.ctx, "user:"+idStr)
	s.cache.Del(s.ctx, "user:list")
	core.Success(c, "删除成功")
}

// Register 注册（改造为依赖注入）
func (s *UserService) Register(c *gin.Context) {
	var user model.User
	if err := c.ShouldBindJSON(&user); err != nil {
		core.Fail(c, http.StatusBadRequest, "参数错误")
		return
	}

	// 使用依赖注入的数据库操作查询用户名是否存在
	count, err := s.db.CountByUsername(user.Username)
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "查询用户名失败")
		return
	}
	if count > 0 {
		core.Fail(c, http.StatusBadRequest, "用户名已经存在")
		return
	}

	user.Role = "user"
	if user.Password == "" {
		core.Fail(c, http.StatusBadRequest, "密码不能为空")
		return
	}

	// 密码加密
	hashPasswd, err := bcrypt.GenerateFromPassword([]byte(user.Password), 10)
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "密码加密失败")
		return
	}
	user.Password = string(hashPasswd)

	// 使用依赖注入的数据库创建操作
	if err := s.db.Create(&user); err != nil {
		core.Fail(c, http.StatusInternalServerError, "注册失败")
		return
	}

	core.Success(c, "注册成功")
}

// Login 登录（改造为依赖注入）
func (s *UserService) Login(c *gin.Context) {
	var req model.User
	if err := c.ShouldBindJSON(&req); err != nil {
		core.Fail(c, http.StatusBadRequest, "参数错误")
		return
	}

	// 使用依赖注入的数据库操作查询用户
	user, err := s.db.FindByUsername(req.Username)
	if err != nil {
		core.Fail(c, http.StatusNotFound, "用户不存在")
		return
	}

	// 密码校验
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		core.Fail(c, http.StatusBadRequest, "密码错误")
		return
	}

	// 生成JWT token
	expirationTime := time.Now().Add(1 * time.Hour) //1小时过期
	// 构造载荷
	claims := &Claims{
		UserId:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	// generate token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.GetJwtKey())
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "生成令牌失效")
		return
	}

	core.Success(c, gin.H{
		"token": tokenString,
		"role":  user.Role,
	})
}

// SearchUser 根据用户名搜索用户（改造为依赖注入）
// 注意：需要在 UserDB 接口中新增 FindByUsernameLike 方法（见下方补充）
func (s *UserService) SearchUser(c *gin.Context) {
	username := c.Query("username")
	if username == "" {
		core.Fail(c, http.StatusBadRequest, "用户名不能为空")
		return
	}

	// 这里需要扩展 UserDB 接口，增加模糊查询方法
	// 临时备注：需要在 repository/user_repo.go 的 UserDB 中添加 FindByUsernameLike 方法
	users, err := s.db.FindByUsernameLike(fmt.Sprintf("%%%s%%", username), 20)
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "搜索用户失败")
		return
	}

	// 格式化返回数据
	var result []gin.H
	for _, user := range users {
		result = append(result, gin.H{
			"id":       user.ID,
			"username": user.Username,
			"role":     user.Role,
		})
	}

	core.Success(c, result)
}

// BatchGetUserRoles 批量获取用户角色（改造为依赖注入）
func (s *UserService) BatchGetUserRoles(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		core.Fail(c, http.StatusBadRequest, "ids不能为空")
		return
	}
	if len(req.IDs) > 100 {
		core.Fail(c, http.StatusBadRequest, "单次最多查询100个用户")
		return
	}

	// 使用依赖注入的数据库操作批量查询用户
	users, err := s.db.FindByIDs(req.IDs)
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "批量查询用户失败")
		return
	}

	roleMap := make(map[uint]string)
	for _, u := range users {
		roleMap[u.ID] = u.Role
	}

	core.Success(c, roleMap)
}
