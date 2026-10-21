package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go-admin/internal/dao"
	"go-admin/internal/domain/model"
	"go-admin/pkg/core"

	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/bcrypt"
)

var ErrUserExists = errors.New("user exists")

// AuthOptions 注册风控 / 演示账号配置（container 从 config 注入，避免 service 依赖 config 包）。
type AuthOptions struct {
	RegisterInviteCode string // 非空时注册必须带邀请码
	DemoEnabled        bool   // 是否开放 /user/demo-login
	DemoUsername       string // 演示账号用户名（默认 demo）
}

// UserService 请求链路 Service：ctx 一律方法级传入，不持有请求级上下文（Context 反模式修复）。
type UserService struct {
	db       dao.UserDB
	cache    dao.UserCache
	jwtCfg   dao.JWTConfig
	jwtKey   []byte
	presence *PresenceService // 在线状态唯一来源（S2-11）
	revoker  *TokenRevocationStore
	authOpts AuthOptions
	// onUserKicked 踢下线回调（container 注入，避免 Service 循环依赖）：
	// 用户权限/角色变更时强制断开其 WS 连接，触发 presence 下线。
	onUserKicked func(userID uint)
}

type Claims struct {
	UserId   uint   `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	// IssuedAtMs 毫秒级签发时间：JWT 标准 iat 只有秒级精度，
	// 改密后「同一秒内签发的旧 token」会漏吊销；加毫秒字段用于精确失效判断。
	IssuedAtMs int64 `json:"iat_ms,omitempty"`
	jwt.RegisteredClaims
}

func NewUserService(db dao.UserDB, cache dao.UserCache, jwtCfg dao.JWTConfig, jwtKey []byte, presence *PresenceService, revoker *TokenRevocationStore, authOpts AuthOptions) *UserService {
	if authOpts.DemoUsername == "" {
		authOpts.DemoUsername = "demo"
	}
	return &UserService{
		db:       db,
		cache:    cache,
		jwtCfg:   jwtCfg,
		jwtKey:   jwtKey,
		presence: presence,
		revoker:  revoker,
		authOpts: authOpts,
	}
}

// SetOnUserKicked 注入踢下线回调（由 container 在 wsService 创建后调用，避免构造期循环依赖）。
func (s *UserService) SetOnUserKicked(fn func(userID uint)) {
	s.onUserKicked = fn
}

func (s *UserService) Register(ctx context.Context, username string, password string, inviteCode string) error {
	if err := s.verifyInviteCode(inviteCode); err != nil {
		return err
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
		Role:     model.RoleUser,
	}
	if err := s.db.Create(&user); err != nil {
		return err
	}

	s.cacheUserProfile(ctx, &user)
	return nil
}

// verifyInviteCode 演示/公网环境注册风控：配置了邀请码就必须带对，否则 403。
func (s *UserService) verifyInviteCode(inviteCode string) error {
	expected := strings.TrimSpace(s.authOpts.RegisterInviteCode)
	if expected == "" {
		return nil
	}
	got := strings.TrimSpace(inviteCode)
	if subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
		return core.NewBizError(http.StatusForbidden, "邀请码错误或缺失")
	}
	return nil
}

// RegisterInviteRequired 前端注册表单据此决定是否展示邀请码输入框。
func (s *UserService) RegisterInviteRequired() bool {
	return strings.TrimSpace(s.authOpts.RegisterInviteCode) != ""
}

// DemoLoginEnabled 前端登录页据此决定是否展示「体验 Demo」入口。
func (s *UserService) DemoLoginEnabled() bool {
	return s.authOpts.DemoEnabled
}

// DemoLogin 免密登录预置演示账号：仅当 demo.enabled=true 且种子账号存在时可用。
// 演示账号由 cmd/seed 创建（密码随机/由运维指定，不对外暴露）。
func (s *UserService) DemoLogin(ctx context.Context) (string, string, error) {
	if !s.authOpts.DemoEnabled {
		return "", "", core.NewBizError(http.StatusNotFound, "演示登录未开启")
	}
	username := s.authOpts.DemoUsername
	if username == "" {
		username = "demo"
	}
	user, err := s.db.FindByUsername(username)
	if err != nil {
		return "", "", core.NewBizError(http.StatusNotFound, "演示账号未初始化，请先在服务端执行 seed 命令")
	}
	token, err := s.issueToken(user)
	if err != nil {
		return "", "", err
	}
	s.cacheUserProfile(ctx, user)
	return token, user.Role, nil
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

// GetUser 返回脱敏后的用户资料（只含 id/username/role/created_at），
// 不直接回传 GORM 模型（避免 DeletedAt 等内部字段、并统一小写字段风格）。
func (s *UserService) GetUser(ctx context.Context, id uint) (map[string]interface{}, error) {
	key := fmt.Sprintf("user:%d", id)
	if val, err := s.cache.Get(ctx, key); err == nil {
		if val == cacheNullValue {
			return nil, core.NewBizError(http.StatusNotFound, "user not found")
		}

		var user model.User
		if err := json.Unmarshal([]byte(val), &user); err == nil {
			return buildSafeUserProfile(&user), nil
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
				return buildSafeUserProfile(&user), nil
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

	return buildSafeUserProfile(user), nil
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

	s.cacheUserProfile(ctx, user) // 更新用户缓存
	if s.onUserKicked != nil {
		s.onUserKicked(id) // 强制断开 WS 连接 → presence 下线
	}
	// 角色/用户名已变化，旧 JWT 里的 claims 不再可信：让该用户此前签发的 token 全部失效
	s.invalidateUserTokens(ctx, id)
	s.deleteUserProfileCache(ctx, id)                             // 删除个人缓存
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
			return core.NewBizError(http.StatusBadRequest, "旧密码不正确")
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

	// 密码已变更：此前签发的 token 全部失效（含改密者当前会话），强制重新登录
	s.invalidateUserTokens(ctx, id)
	if s.onUserKicked != nil {
		s.onUserKicked(id)
	}
	_ = s.cache.Del(ctx, fmt.Sprintf("user:%d", id), "user:list")
	return nil
}

// invalidateUserTokens 写入用户级失效时间戳：早于该时间签发的 token 立即 401。
func (s *UserService) invalidateUserTokens(ctx context.Context, userID uint) {
	if s.revoker == nil || userID == 0 {
		return
	}
	if err := s.revoker.InvalidateUserBefore(ctx, userID, time.Now()); err != nil {
		core.Z().Errorf("invalidate user tokens failed user_id=%d: %v", userID, err)
	}
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
		// 不暴露「用户是否存在」，统一返回同一文案，避免账号枚举
		return "", "", core.NewBizError(http.StatusBadRequest, "用户名或密码错误")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return "", "", core.NewBizError(http.StatusBadRequest, "用户名或密码错误")
	}

	tokenString, err := s.issueToken(user)
	if err != nil {
		return "", "", err
	}

	s.cacheUserProfile(ctx, user)
	// 在线状态由 WS 连接管理（presence），登录成功不写在线标记，避免"登录≠连接"的假在线。
	return tokenString, user.Role, nil
}

// issueToken 统一签发 JWT（登录 / 演示登录共用）。
func (s *UserService) issueToken(user *model.User) (string, error) {
	now := time.Now()
	expirationTime := now.Add(s.jwtCfg.GetExpireHours())
	claims := &Claims{
		UserId:     user.ID,
		Username:   user.Username,
		Role:       user.Role,
		IssuedAtMs: now.UnixMilli(),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.getJWTKey())
	if err != nil {
		return "", core.NewBizError(http.StatusInternalServerError, "sign token failed")
	}
	return tokenString, nil
}

func (s *UserService) Logout(ctx context.Context, userID uint, token string, expiresAt time.Time) error {
	if userID == 0 {
		return core.NewBizError(http.StatusUnauthorized, "missing user identity")
	}
	// 服务端主动失效：把当前 token 指纹写入 Redis 黑名单，logout 后同一 token 立即 401。
	if s.revoker != nil && token != "" {
		if err := s.revoker.Revoke(ctx, token, userID, expiresAt); err != nil {
			core.Z().Errorf("revoke token failed user_id=%d: %v", userID, err)
			return core.NewBizError(http.StatusInternalServerError, "logout failed")
		}
	}
	// 断开 WS 连接并触发 presence 下线清理（前端已经 close，这里是服务端兜底）
	if s.onUserKicked != nil {
		s.onUserKicked(userID)
	}
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
