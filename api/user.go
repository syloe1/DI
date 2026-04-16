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
	db                repository.UserDB
	cache             repository.UserCache
	jwtCfg            repository.JWTConfig
	jwtKey            []byte
	ctx               context.Context
	interactExtension userInteractExtension
	socialExtension   userSocialExtension
	messageExtension  userMessageExtension
	wsExtension       userWSExtension
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

type userInteractExtension interface {
	GetInteractCount(c *gin.Context)
	ToggleLike(c *gin.Context)
	ToggleDislike(c *gin.Context)
	ToggleCollect(c *gin.Context)
	Share(c *gin.Context)
	GetInteractStatus(c *gin.Context)
}

type userSocialExtension interface {
	FollowUser(c *gin.Context)
	BlockUser(c *gin.Context)
	GetRelationStatus(c *gin.Context)
	GetFollowList(c *gin.Context)
	GetFollowerList(c *gin.Context)
	GetBlockList(c *gin.Context)
}

type userMessageExtension interface {
	GetConversations(c *gin.Context)
	GetMessageList(c *gin.Context)
	SendMessage(c *gin.Context)
}

type userWSExtension interface {
	HandleWebSocket(c *gin.Context)
}

func (s *UserService) SetInteractExtension(extension userInteractExtension) {
	s.interactExtension = extension
}

func (s *UserService) SetSocialExtension(extension userSocialExtension) {
	s.socialExtension = extension
}

func (s *UserService) SetMessageExtension(extension userMessageExtension) {
	s.messageExtension = extension
}

func (s *UserService) SetWSExtension(extension userWSExtension) {
	s.wsExtension = extension
}

type Claims struct {
	UserId   uint   `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

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

	if err := s.db.Create(&user); err != nil {
		core.Fail(c, http.StatusInternalServerError, "创建用户失败")
		return
	}

	_ = s.cache.Del(s.ctx, "user:list")
	core.Success(c, "添加成功")
}

func (s *UserService) GetUserList(c *gin.Context) {
	role := c.GetString("role")
	if role != "admin" && role != "superadmin" {
		core.Fail(c, http.StatusForbidden, "无权限查看用户列表")
		return
	}

	key := "user:list"
	if val, err := s.cache.Get(s.ctx, key); err == nil {
		if val == cacheNullValue {
			core.Success(c, []gin.H{})
			return
		}

		var safeList []gin.H
		if err := json.Unmarshal([]byte(val), &safeList); err == nil {
			core.Success(c, safeList)
			return
		}
	}

	lockKey := "lock:" + key
	locked, err := s.cache.SetNX(s.ctx, lockKey, "1", cacheLockTTL)
	if err == nil && !locked {
		if val, ok := spinWaitCache(s.cache, s.ctx, key); ok {
			if val == cacheNullValue {
				core.Success(c, []gin.H{})
				return
			}
			var safeList []gin.H
			if err := json.Unmarshal([]byte(val), &safeList); err == nil {
				core.Success(c, safeList)
				return
			}
		}
	}
	if locked {
		defer s.cache.Del(s.ctx, lockKey)
	}

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

	if len(safeList) == 0 {
		_ = s.cache.Set(s.ctx, key, cacheNullValue, cacheNullTTL)
		core.Success(c, safeList)
		return
	}

	if listJSON, err := json.Marshal(safeList); err == nil {
		_ = s.cache.Set(s.ctx, key, string(listJSON), jitterTTL(defaultCacheTTL))
	}

	core.Success(c, safeList)
}

func (s *UserService) GetUser(c *gin.Context) {
	idStr := c.Param("id")

	var id uint
	_, err := fmt.Sscanf(idStr, "%d", &id)
	if err != nil {
		core.Fail(c, http.StatusBadRequest, "用户ID格式错误")
		return
	}

	key := "user:" + idStr
	if val, err := s.cache.Get(s.ctx, key); err == nil {
		if val == cacheNullValue {
			core.Fail(c, http.StatusNotFound, "用户不存在")
			return
		}
		var user model.User
		if err := json.Unmarshal([]byte(val), &user); err == nil {
			core.Success(c, user)
			return
		}
	}

	lockKey := "lock:" + key
	locked, err := s.cache.SetNX(s.ctx, lockKey, "1", cacheLockTTL)
	if err == nil && !locked {
		if val, ok := spinWaitCache(s.cache, s.ctx, key); ok {
			if val == cacheNullValue {
				core.Fail(c, http.StatusNotFound, "用户不存在")
				return
			}
			var user model.User
			if err := json.Unmarshal([]byte(val), &user); err == nil {
				core.Success(c, user)
				return
			}
		}
	}
	if locked {
		defer s.cache.Del(s.ctx, lockKey)
	}

	user, err := s.db.FindByID(id)
	if err != nil {
		_ = s.cache.Set(s.ctx, key, cacheNullValue, cacheNullTTL)
		core.Fail(c, http.StatusNotFound, "用户不存在")
		return
	}

	if userJSON, err := json.Marshal(user); err == nil {
		_ = s.cache.Set(s.ctx, key, string(userJSON), jitterTTL(defaultCacheTTL))
	}

	core.Success(c, user)
}

func (s *UserService) UpdateUser(c *gin.Context) {
	idStr := c.Param("id")
	currentUserID := c.GetUint("user_id")
	currentRole := c.GetString("role")

	var id uint
	_, err := fmt.Sscanf(idStr, "%d", &id)
	if err != nil {
		core.Fail(c, http.StatusBadRequest, "用户ID格式错误")
		return
	}

	user, err := s.db.FindByID(id)
	if err != nil {
		core.Fail(c, http.StatusNotFound, "用户不存在")
		return
	}

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

	if err := s.db.Update(user); err != nil {
		core.Fail(c, http.StatusInternalServerError, "修改用户信息失败")
		return
	}

	_ = s.cache.Del(s.ctx, "user:"+idStr, "user:list")
	core.Success(c, "修改成功")
}

func (s *UserService) ChangePassword(c *gin.Context) {
	idStr := c.Param("id")
	currentUserID := c.GetUint("userID")
	currentRole := c.GetString("role")

	var id uint
	_, err := fmt.Sscanf(idStr, "%d", &id)
	if err != nil {
		core.Fail(c, http.StatusBadRequest, "用户ID格式错误")
		return
	}

	user, err := s.db.FindByID(id)
	if err != nil {
		core.Fail(c, http.StatusNotFound, "用户不存在")
		return
	}

	if id != currentUserID {
		if currentRole == "superadmin" {
		} else if currentRole == "admin" {
			if user.Role == "admin" || user.Role == "superadmin" {
				core.Fail(c, http.StatusForbidden, "管理员只能修改普通用户的密码")
				return
			}
		} else {
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

	if currentRole != "admin" && currentRole != "superadmin" {
		err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.OldPassword))
		if err != nil {
			core.Fail(c, http.StatusBadRequest, "原密码错误")
			return
		}
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "密码加密失败")
		return
	}

	user.Password = string(hashedPassword)
	if err := s.db.Update(user); err != nil {
		core.Fail(c, http.StatusInternalServerError, "修改密码失败")
		return
	}

	_ = s.cache.Del(s.ctx, "user:"+idStr, "user:list")
	core.Success(c, "密码修改成功")
}

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

	_, err = s.db.FindByID(id)
	if err != nil {
		core.Fail(c, http.StatusNotFound, "用户不存在")
		return
	}

	if err := s.db.Delete(id); err != nil {
		core.Fail(c, http.StatusInternalServerError, "删除用户失败")
		return
	}

	_ = s.cache.Del(s.ctx, "user:"+idStr, "user:list")
	core.Success(c, "删除成功")
}

func (s *UserService) Register(c *gin.Context) {
	var user model.User
	if err := c.ShouldBindJSON(&user); err != nil {
		core.Fail(c, http.StatusBadRequest, "参数错误")
		return
	}

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

	hashPasswd, err := bcrypt.GenerateFromPassword([]byte(user.Password), 10)
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "密码加密失败")
		return
	}
	user.Password = string(hashPasswd)

	if err := s.db.Create(&user); err != nil {
		core.Fail(c, http.StatusInternalServerError, "注册失败")
		return
	}

	core.Success(c, "注册成功")
}

func (s *UserService) Login(c *gin.Context) {
	var req model.User
	if err := c.ShouldBindJSON(&req); err != nil {
		core.Fail(c, http.StatusBadRequest, "参数错误")
		return
	}

	user, err := s.db.FindByUsername(req.Username)
	if err != nil {
		core.Fail(c, http.StatusNotFound, "用户不存在")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		core.Fail(c, http.StatusBadRequest, "密码错误")
		return
	}

	expirationTime := time.Now().Add(1 * time.Hour)
	claims := &Claims{
		UserId:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

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

func (s *UserService) SearchUser(c *gin.Context) {
	username := c.Query("username")
	if username == "" {
		core.Fail(c, http.StatusBadRequest, "用户名不能为空")
		return
	}

	users, err := s.db.FindByUsernameLike(fmt.Sprintf("%%%s%%", username), 20)
	if err != nil {
		core.Fail(c, http.StatusInternalServerError, "搜索用户失败")
		return
	}

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

func (s *UserService) HandleWebSocket(c *gin.Context) {
	if s.wsExtension == nil {
		core.Fail(c, http.StatusNotImplemented, "WebSocket功能暂未实现")
		return
	}
	s.wsExtension.HandleWebSocket(c)
}

func (s *UserService) GetInteractCount(c *gin.Context) {
	if s.interactExtension == nil {
		core.Fail(c, http.StatusNotImplemented, "互动统计功能暂未实现")
		return
	}
	s.interactExtension.GetInteractCount(c)
}

func (s *UserService) ToggleLike(c *gin.Context) {
	if s.interactExtension == nil {
		core.Fail(c, http.StatusNotImplemented, "点赞功能暂未实现")
		return
	}
	s.interactExtension.ToggleLike(c)
}

func (s *UserService) ToggleDislike(c *gin.Context) {
	if s.interactExtension == nil {
		core.Fail(c, http.StatusNotImplemented, "点踩功能暂未实现")
		return
	}
	s.interactExtension.ToggleDislike(c)
}

func (s *UserService) ToggleCollect(c *gin.Context) {
	if s.interactExtension == nil {
		core.Fail(c, http.StatusNotImplemented, "收藏功能暂未实现")
		return
	}
	s.interactExtension.ToggleCollect(c)
}

func (s *UserService) Share(c *gin.Context) {
	if s.interactExtension == nil {
		core.Fail(c, http.StatusNotImplemented, "分享功能暂未实现")
		return
	}
	s.interactExtension.Share(c)
}

func (s *UserService) GetInteractStatus(c *gin.Context) {
	if s.interactExtension == nil {
		core.Fail(c, http.StatusNotImplemented, "互动状态功能暂未实现")
		return
	}
	s.interactExtension.GetInteractStatus(c)
}

func (s *UserService) FollowUser(c *gin.Context) {
	if s.socialExtension == nil {
		core.Fail(c, http.StatusNotImplemented, "关注功能暂未实现")
		return
	}
	s.socialExtension.FollowUser(c)
}

func (s *UserService) BlockUser(c *gin.Context) {
	if s.socialExtension == nil {
		core.Fail(c, http.StatusNotImplemented, "拉黑功能暂未实现")
		return
	}
	s.socialExtension.BlockUser(c)
}

func (s *UserService) GetRelationStatus(c *gin.Context) {
	if s.socialExtension == nil {
		core.Fail(c, http.StatusNotImplemented, "关系状态功能暂未实现")
		return
	}
	s.socialExtension.GetRelationStatus(c)
}

func (s *UserService) GetFollowList(c *gin.Context) {
	if s.socialExtension == nil {
		core.Fail(c, http.StatusNotImplemented, "关注列表功能暂未实现")
		return
	}
	s.socialExtension.GetFollowList(c)
}

func (s *UserService) GetFollowerList(c *gin.Context) {
	if s.socialExtension == nil {
		core.Fail(c, http.StatusNotImplemented, "粉丝列表功能暂未实现")
		return
	}
	s.socialExtension.GetFollowerList(c)
}

func (s *UserService) GetBlockList(c *gin.Context) {
	if s.socialExtension == nil {
		core.Fail(c, http.StatusNotImplemented, "拉黑列表功能暂未实现")
		return
	}
	s.socialExtension.GetBlockList(c)
}

func (s *UserService) GetConversations(c *gin.Context) {
	if s.messageExtension == nil {
		core.Fail(c, http.StatusNotImplemented, "会话列表功能暂未实现")
		return
	}
	s.messageExtension.GetConversations(c)
}

func (s *UserService) GetMessageList(c *gin.Context) {
	if s.messageExtension == nil {
		core.Fail(c, http.StatusNotImplemented, "消息列表功能暂未实现")
		return
	}
	s.messageExtension.GetMessageList(c)
}

func (s *UserService) SendMessage(c *gin.Context) {
	if s.messageExtension == nil {
		core.Fail(c, http.StatusNotImplemented, "发送消息功能暂未实现")
		return
	}
	s.messageExtension.SendMessage(c)
}