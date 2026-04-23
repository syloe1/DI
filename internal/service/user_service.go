package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"go-admin/internal/dao"
	"go-admin/internal/domain/model"
	"go-admin/pkg/core"

	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/bcrypt"
)

var ErrUserExists = errors.New("user exists")

type UserService struct {
	db     dao.UserDB
	cache  dao.UserCache
	jwtCfg dao.JWTConfig
	jwtKey []byte
	ctx    context.Context
}

type Claims struct {
	UserId   uint   `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

func NewUserService(db dao.UserDB, cache dao.UserCache, jwtCfg dao.JWTConfig, jwtKey []byte, ctx context.Context) *UserService {
	return &UserService{
		db:     db,
		cache:  cache,
		jwtCfg: jwtCfg,
		jwtKey: jwtKey,
		ctx:    ctx,
	}
}

func (s *UserService) Register(username string, password string) error {
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

	s.cacheUserProfile(&user)
	return nil
}

func (s *UserService) AddUser(actorRole string, username string, password string, role string) error {
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

	s.cacheUserProfile(&user)
	_ = s.cache.Del(s.ctx, "user:list")
	return nil
}

func (s *UserService) GetUserList(actorRole string) ([]map[string]interface{}, error) {
	if actorRole != model.RoleAdmin && actorRole != model.RoleSuperAdmin {
		return nil, core.NewBizError(http.StatusForbidden, "no permission to view user list")
	}

	key := "user:list"
	if val, err := s.cache.Get(s.ctx, key); err == nil {
		if val == cacheNullValue {
			return []map[string]interface{}{}, nil
		}

		var safeList []map[string]interface{}
		if err := json.Unmarshal([]byte(val), &safeList); err == nil {
			return safeList, nil
		}
	}

	lockKey := "lock:" + key
	locked, err := s.cache.SetNX(s.ctx, lockKey, "1", cacheLockTTL)
	if err == nil && !locked {
		if val, ok := spinWaitCache(s.cache, s.ctx, key); ok {
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
		defer s.cache.Del(s.ctx, lockKey)
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
		_ = s.cache.Set(s.ctx, key, cacheNullValue, cacheNullTTL)
		return safeList, nil
	}

	if listJSON, err := json.Marshal(safeList); err == nil {
		_ = s.cache.Set(s.ctx, key, string(listJSON), jitterTTL(defaultCacheTTL))
	}

	return safeList, nil
}

func (s *UserService) GetUser(id uint) (*model.User, error) {
	key := fmt.Sprintf("user:%d", id)
	if val, err := s.cache.Get(s.ctx, key); err == nil {
		if val == cacheNullValue {
			return nil, core.NewBizError(http.StatusNotFound, "user not found")
		}

		var user model.User
		if err := json.Unmarshal([]byte(val), &user); err == nil {
			return &user, nil
		}
	}

	lockKey := "lock:" + key
	locked, err := s.cache.SetNX(s.ctx, lockKey, "1", cacheLockTTL)
	if err == nil && !locked {
		if val, ok := spinWaitCache(s.cache, s.ctx, key); ok {
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
		defer s.cache.Del(s.ctx, lockKey)
	}

	user, err := s.db.FindByID(id)
	if err != nil {
		_ = s.cache.Set(s.ctx, key, cacheNullValue, cacheNullTTL)
		return nil, core.NewBizError(http.StatusNotFound, "user not found")
	}

	if userJSON, err := json.Marshal(user); err == nil {
		_ = s.cache.Set(s.ctx, key, string(userJSON), jitterTTL(defaultCacheTTL))
	}

	return user, nil
}

func (s *UserService) UpdateUser(id uint, currentUserID uint, currentRole string, username string, role string) error {
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

	s.cacheUserProfile(user)                                        // 更新用户缓存
	_ = s.cache.SRem(s.ctx, OnlineUsersKey, id)                     // 把用户踢下线（强制重新登录拿最新权限）
	s.deleteUserProfileCache(id)                                    // 删除个人缓存
	_ = s.cache.Del(s.ctx, fmt.Sprintf("user:%d", id), "user:list") // 删除用户列表缓存
	return nil
}

func (s *UserService) ChangePassword(id uint, currentUserID uint, currentRole string, oldPassword string, newPassword string) error {
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

	_ = s.cache.Del(s.ctx, fmt.Sprintf("user:%d", id), "user:list")
	return nil
}

func (s *UserService) DeleteUser(actorRole string, id uint) error {
	if actorRole != model.RoleAdmin && actorRole != model.RoleSuperAdmin {
		return core.NewBizError(http.StatusForbidden, "no permission to delete user")
	}

	if _, err := s.db.FindByID(id); err != nil {
		return core.NewBizError(http.StatusNotFound, "user not found")
	}

	if err := s.db.Delete(id); err != nil {
		return core.NewBizError(http.StatusInternalServerError, "delete user failed")
	}

	_ = s.cache.Del(s.ctx, fmt.Sprintf("user:%d", id), "user:list")
	return nil
}

func (s *UserService) Login(username string, password string) (string, string, error) {
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

	s.cacheUserProfile(user)
	_ = s.cache.SAdd(s.ctx, OnlineUsersKey, user.ID)
	return tokenString, user.Role, nil
}

func (s *UserService) Logout(userID uint) error {
	if userID == 0 {
		return core.NewBizError(http.StatusUnauthorized, "missing user identity")
	}
	if err := s.cache.SRem(s.ctx, OnlineUsersKey, userID); err != nil {
		return core.NewBizError(http.StatusInternalServerError, "logout failed")
	}
	return nil
}

func (s *UserService) SearchUser(username string) ([]map[string]interface{}, error) {
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
		if online, err := s.cache.SIsMember(s.ctx, OnlineUsersKey, user.ID); err == nil {
			item["is_online"] = online
		}
		result = append(result, item)
	}

	return result, nil
}

func (s *UserService) GetUserOnlineStatus(id uint) (map[string]interface{}, error) {
	online, err := s.cache.SIsMember(s.ctx, OnlineUsersKey, id)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get online status failed")
	}

	return map[string]interface{}{
		"user_id":   id,
		"is_online": online,
	}, nil
}

func (s *UserService) GetOnlineUsers() (map[string]interface{}, error) {
	members, err := s.cache.SMembers(s.ctx, OnlineUsersKey)
	if err != nil {
		return nil, core.NewBizError(http.StatusInternalServerError, "get online users failed")
	}

	users := make([]map[string]interface{}, 0, len(members))
	for _, member := range members {
		id64, err := strconv.ParseUint(member, 10, 64)
		if err != nil {
			continue
		}
		id := uint(id64)

		if profile, ok := s.getCachedUserProfile(id); ok {
			users = append(users, profile)
			continue
		}

		user, err := s.db.FindByID(id)
		if err != nil {
			continue
		}
		s.cacheUserProfile(user)
		profile := buildSafeUserProfile(user)
		profile["is_online"] = true
		users = append(users, profile)
	}

	return map[string]interface{}{
		"count": len(users),
		"list":  users,
	}, nil
}

func (s *UserService) BatchGetUserRoles(ids []uint) (map[uint]string, error) {
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
