package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"go-admin/internal/dao"
	"go-admin/internal/domain/model"
	"go-admin/pkg/core"

	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/bcrypt"
)

var ErrUserExists = errors.New("user exists")

// UserService 请求链路 Service：ctx 一律方法级传入，不持有请求级上下文（Context 反模式修复）。
type UserService struct {
	db     dao.UserDB
	cache  dao.UserCache
	jwtCfg dao.JWTConfig
	jwtKey []byte
	presence *PresenceService // 在线状态唯一来源（S2-11）
	// onUserKicked 踢下线回调（container 注入，避免 Service 循环依赖）：
	// 用户权限/角色变更时强制断开其 WS 连接，触发 presence 下线。
	onUserKicked func(userID uint)
}

type Claims struct {
	UserId   uint   `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

func NewUserService(db dao.UserDB, cache dao.UserCache, jwtCfg dao.JWTConfig, jwtKey []byte, presence *PresenceService) *UserService {
	return &UserService{
		db:       db,
		cache:    cache,
		jwtCfg:   jwtCfg,
		jwtKey:   jwtKey,
		presence: presence,
	}
}

// SetOnUserKicked 注入踢下线回调（由 container 在 wsService 创建后调用，避免构造期循环依赖）。
func (s *UserService) SetOnUserKicked(fn func(userID uint)) {
	s.onUserKicked = fn
}

func (s *UserService) Register(ctx context.Context, username string, password string) error {
	if err := s.ensureUsernameAvailable(username); err != nil {
		return err
	}

	hashPasswd, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return err
	}

	user := model.User{
		Username: username,
		Password: string(hashPasswd),
		Role:     model.RoleUser,
	}
	if err := s.db.Create(&user); err != nil {
		return err
	}

	s.cacheUserProfile(ctx, &user)
	return nil
}

func (s *UserService) AddUser(ctx context.Context, actorRole string, username string, password string, role string) error {
	if actorRole != model.RoleAdmin && actorRole != model.RoleSuperAdmin {
		return core.NewBizError(http.StatusForbidden, "no permission to add user")
	}
	if role == "" {
		role = model.RoleUser
	}
	if err := s.ensureUsernameAvailable(username); err != nil {
		return err
	}

	hashPasswd, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return err
	}

	user := model.User{
		Username: username,
		Password: string(hashPasswd),
		Role:     role,
	}
	if err := s.db.Create(&user); err != nil {
		return err
	}

	s.cacheUserProfile(ctx, &user)
	_ = s.cache.Del(ctx, "user:list")
	return nil
}

func (s *UserService) GetUserList(ctx context.Context, actorRole string) ([]map[string]interface{}, error) {
	if actorRole != model.RoleAdmin && actorRole != model.RoleSuperAdmin {
		return nil, core.NewBizError(http.StatusForbidden, "no permission to view user list")
	}

	key := "user:list"
	if val, err := s.cache.Get(ctx, key); err == nil {
		if val == cacheNullValue {
			return []map[string]interface{}{}, nil
		}

		var safeList []map[string]interface{}
		if err := json.Unmarshal([]byte(val), &safeList); err == nil {
			return safeList, nil
		}
	}

	lockKey := "lock:" + key
	locked, err := s.cache.SetNX(ctx, lockKey, "1", cacheLockTTL)
	if err == nil && !locked {
		if val, ok := spinWaitCache(s.cache, ctx, key); ok {
			if val == cacheNullValue {
				return []map[string]interface{}{}, nil
			}

			var safeList []map[string]interface{}
			if err := json.Unmarshal([]byte(val), &safeList); err == nil {
				return safeList, nil
			}
		}
	}
	if locked {
		defer s.cache.Del(ctx, lockKey)
	}

	userList, err := s.db.FindAll()
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "query user list failed")
	}

	safeList := make([]map[string]interface{}, len(userList))
	for i, u := range userList {
		safeList[i] = map[string]interface{}{
			"id":         u.ID,
			"username":   u.Username,
			"role":       u.Role,
			"created_at": u.CreatedAt,
		}
	}

	if len(safeList) == 0 {
		_ = s.cache.Set(ctx, key, cacheNullValue, cacheNullTTL)
		return safeList, nil
	}

	if listJSON, err := json.Marshal(safeList); err == nil {
		_ = s.cache.Set(ctx, key, string(listJSON), jitterTTL(defaultCacheTTL))
	}

	return safeList, nil
}

func (s *UserService) GetUser(ctx context.Context, id uint) (*model.User, error) {
	key := fmt.Sprintf("user:%d", id)
	if val, err := s.cache.Get(ctx, key); err == nil {
		if val == cacheNullValue {
			return nil, core.NewBizError(http.StatusNotFound, "user not found")
		}

		var user model.User
		if err := json.Unmarshal([]byte(val), &user); err == nil {
			return &user, nil
		}
	}

	lockKey := "lock:" + key
	locked, err := s.cache.SetNX(ctx, lockKey, "1", cacheLockTTL)
	if err == nil && !locked {
		if val, ok := spinWaitCache(s.cache, ctx, key); ok {
			if val == cacheNullValue {
				return nil, core.NewBizError(http.StatusNotFound, "user not found")
			}

			var user model.User
			if err := json.Unmarshal([]byte(val), &user); err == nil {
				return &user, nil
			}
		}
	}
	if locked {
		defer s.cache.Del(ctx, lockKey)
	}

	user, err := s.db.FindByID(id)
	if err != nil {
		_ = s.cache.Set(ctx, key, cacheNullValue, cacheNullTTL)
		return nil, core.NewBizError(http.StatusNotFound, "user not found")
	}

	if userJSON, err := json.Marshal(user); err == nil {
		_ = s.cache.Set(ctx, key, string(userJSON), jitterTTL(defaultCacheTTL))
	}

	return user, nil
}

func (s *UserService) UpdateUser(ctx context.Context, id uint, currentUserID uint, currentRole string, username string, role string) error {
	user, err := s.db.FindByID(id)
	if err != nil {
		return core.NewBizError(http.StatusNotFound, "user not found")
	}

	if user.ID != currentUserID && currentRole != model.RoleAdmin && currentRole != model.RoleSuperAdmin {
		return core.NewBizError(http.StatusForbidden, "no permission to update other users")
	}

	if username != "" {
		user.Username = username
	}
	if role != "" {
		if currentRole == model.RoleSuperAdmin {
			user.Role = role
		} else if currentRole == model.RoleAdmin && role == model.RoleUser {
			user.Role = role
		}
	}

	if err := s.db.Update(user); err != nil {
		return core.NewBizError(http.StatusInternalServerError, "update user failed")
	}

	s.cacheUserProfile(ctx, user)     // 更新用户缓存
	if s.onUserKicked != nil {
		s.onUserKicked(id) // 强制断开 WS 连接 → presence 下线
	}
	s.deleteUserProfileCache(ctx, id) // 删除个人缓存
	_ = s.cache.Del(ctx, fmt.Sprintf("user:%d", id), "user:list") // 删除用户列表缓存
	return nil
}

func (s *UserService) ChangePassword(ctx context.Context, id uint, currentUserID uint, currentRole string, oldPassword string, newPassword string) error {
	user, err := s.db.FindByID(id)
	if err != nil {
		return core.NewBizError(http.StatusNotFound, "user not found")
	}

	if id != currentUserID {
		if currentRole == model.RoleSuperAdmin {
		} else if currentRole == model.RoleAdmin {
			if user.Role == model.RoleAdmin || user.Role == model.RoleSuperAdmin {
				return core.NewBizError(http.StatusForbidden, "admin can only change normal user passwords")
			}
		} else {
			return core.NewBizError(http.StatusForbidden, "can only change your own password")
		}
	}

	if currentRole != model.RoleAdmin && currentRole != model.RoleSuperAdmin {
		if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(oldPassword)); err != nil {
			return core.NewBizError(http.StatusBadRequest, "old password is incorrect")
		}
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return core.NewBizError(http.StatusInternalServerError, "password hashing failed")
	}

	user.Password = string(hashedPassword)
	if err := s.db.Update(user); err != nil {
		return core.NewBizError(http.StatusInternalServerError, "change password failed")
	}

	_ = s.cache.Del(ctx, fmt.Sprintf("user:%d", id), "user:list")
	return nil
}

func (s *UserService) DeleteUser(ctx context.Context, actorRole string, id uint) error {
	if actorRole != model.RoleAdmin && actorRole != model.RoleSuperAdmin {
		return core.NewBizError(http.StatusForbidden, "no permission to delete user")
	}

	if _, err := s.db.FindByID(id); err != nil {
		return core.NewBizError(http.StatusNotFound, "user not found")
	}

	if err := s.db.Delete(id); err != nil {
		return core.NewBizError(http.StatusInternalServerError, "delete user failed")
	}

	_ = s.cache.Del(ctx, fmt.Sprintf("user:%d", id), "user:list")
	return nil
}

func (s *UserService) Login(ctx context.Context, username string, password string) (string, string, error) {
	user, err := s.db.FindByUsername(username)
	if err != nil {
		return "", "", core.NewBizError(http.StatusNotFound, "user not found")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return "", "", core.NewBizError(http.StatusBadRequest, "username or password is incorrect")
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
	tokenString, err := token.SignedString(s.getJWTKey())
	if err != nil {
		return "", "", core.NewBizError(http.StatusInternalServerError, "sign token failed")
	}

	s.cacheUserProfile(ctx, user)
	// 在线状态由 WS 连接管理（presence），登录成功不写在线标记，避免"登录≠连接"的假在线。
	return tokenString, user.Role, nil
}

func (s *UserService) Logout(ctx context.Context, userID uint) error {
	if userID == 0 {
		return core.NewBizError(http.StatusUnauthorized, "missing user identity")
	}
	// 在线状态由 WS 断开时通过 presence 清理；Logout 仅作 JWT 失效语义（前端丢弃 token）。
	return nil
}

func (s *UserService) SearchUser(ctx context.Context, username string) ([]map[string]interface{}, error) {
	if username == "" {
		return nil, core.NewBizError(http.StatusBadRequest, "username is required")
	}

	users, err := s.db.FindByUsernameLike(fmt.Sprintf("%%%s%%", username), 20)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "search user failed")
	}

	result := make([]map[string]interface{}, 0, len(users))
	for _, user := range users {
		item := map[string]interface{}{
			"id":       user.ID,
			"username": user.Username,
			"role":     user.Role,
		}
		if online, err := s.presence.IsOnline(ctx, user.ID); err == nil {
			item["is_online"] = online
		}
		result = append(result, item)
	}

	return result, nil
}

func (s *UserService) GetUserOnlineStatus(ctx context.Context, id uint) (map[string]interface{}, error) {
	online, err := s.presence.IsOnline(ctx, id)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get online status failed")
	}

	return map[string]interface{}{
		"user_id":   id,
		"is_online": online,
	}, nil
}

func (s *UserService) GetOnlineUsers(ctx context.Context) (map[string]interface{}, error) {
	ids, err := s.presence.GetOnlineUserIDs(ctx)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get online users failed")
	}

	users := make([]map[string]interface{}, 0, len(ids))
	for _, id := range ids {
		if profile, ok := s.getCachedUserProfile(ctx, id); ok {
			users = append(users, profile)
			continue
		}

		user, err := s.db.FindByID(id)
		if err != nil {
			continue
		}
		s.cacheUserProfile(ctx, user)
		profile := buildSafeUserProfile(user)
		profile["is_online"] = true
		users = append(users, profile)
	}

	return map[string]interface{}{
		"count": len(users),
		"list":  users,
	}, nil
}
func (s *UserService) GetUserRole(ctx context.Context, id uint) (string, error) {
	user, err := s.db.FindByID(id)
	if err != nil {
		return "", core.NewBizError(http.StatusNotFound, "user not found")
	}
	return user.Role, nil
}
func (s *UserService) BatchGetUserRoles(ctx context.Context, ids []uint) (map[uint]string, error) {
	if len(ids) == 0 {
		return nil, core.NewBizError(http.StatusBadRequest, "ids cannot be empty")
	}
	if len(ids) > 100 {
		return nil, core.NewBizError(http.StatusBadRequest, "at most 100 ids per request")
	}

	users, err := s.db.FindByIDs(ids)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "batch query user roles failed")
	}

	roleMap := make(map[uint]string, len(users))
	for _, u := range users {
		roleMap[u.ID] = u.Role
	}
	return roleMap, nil
}

func (s *UserService) BatchPutUserRoles(ctx context.Context, currentUserID uint, currentRole string, ids []uint, role string) error {
	// 基础参数校验
	if len(ids) == 0 {
		return core.NewBizError(http.StatusBadRequest, "ids cannot be empty")
	}
	if len(ids) > 100 {
		return core.NewBizError(http.StatusBadRequest, "at most 100 ids per request")
	}
	if currentUserID == 0 {
		return core.NewBizError(http.StatusUnauthorized, "请先登录")
	}

	// 1. 角色权限校验
	switch currentRole {
	case model.RoleUser:
		return core.NewBizError(http.StatusForbidden, "无权限修改角色")

	case model.RoleAdmin:
		// 管理员禁止设置超级管理员
		if role == model.RoleSuperAdmin {
			return core.NewBizError(http.StatusForbidden, "管理员无法设置超级管理员")
		}
		// 校验目标角色合法
		if role != model.RoleUser && role != model.RoleAdmin {
			return core.NewBizError(http.StatusBadRequest, "无效角色类型")
		}

	case model.RoleSuperAdmin:
		// 超管允许所有合法角色
		if role != model.RoleUser && role != model.RoleAdmin && role != model.RoleSuperAdmin {
			return core.NewBizError(http.StatusBadRequest, "无效角色类型")
		}

	default:
		return core.NewBizError(http.StatusBadRequest, "当前账号角色异常")
	}

	// 2. 禁止修改自己的角色
	for _, id := range ids {
		if id == currentUserID {
			return core.NewBizError(http.StatusForbidden, "不允许修改自身角色")
		}
	}

	// 3. 管理员额外防护：不能修改已存在的超级管理员账号
	if currentRole == model.RoleAdmin {
		users, err := s.db.FindByIDs(ids)
		if err != nil {
			return core.NewBizError(http.StatusInternalServerError, "查询用户失败")
		}
		for _, u := range users {
			if u.Role == model.RoleSuperAdmin {
				return core.NewBizError(http.StatusForbidden, "无权修改超级管理员账号")
			}
		}
	}

	// 执行批量更新
	err := s.db.UpdateRoleByIDs(ids, role)
	if err != nil {
		return core.NewBizError(http.StatusInternalServerError, "更新角色失败")
	}
	return nil
}
func (s *UserService) ensureUsernameAvailable(username string) error {
	count, err := s.db.CountByUsername(username)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrUserExists
	}
	return nil
}

func (s *UserService) getJWTKey() []byte {
	if len(s.jwtKey) > 0 {
		return s.jwtKey
	}
	if s.jwtCfg != nil {
		return s.jwtCfg.GetSecret()
	}
	return nil
}
