# Go-Admin 依赖注入实现示例

## 依赖注入概念

依赖注入（Dependency Injection）是一种设计模式，通过将依赖关系从代码内部移动到外部来降低耦合度。

### 核心优势：
- **解耦合**：组件之间不直接依赖具体实现
- **可测试性**：便于单元测试和模拟
- **可维护性**：易于替换和扩展组件
- **单一职责**：每个组件专注于自己的功能

## 当前项目依赖注入结构

### 1. 接口定义 (`internal/repository/user_repo.go`)

```go
// 数据库操作接口
type UserDB interface {
    Create(user *model.User) error
    FindByID(id uint) (*model.User, error)
    FindByUsername(username string) (*model.User, error)
    Update(user *model.User) error
    Delete(id uint) error
    FindAll() ([]model.User, error)
    FindByIDs(ids []uint) ([]model.User, error)
    CountByUsername(username string) (int64, error)
}

// 缓存操作接口
type UserCache interface {
    Get(ctx context.Context, key string) (string, error)
    Set(ctx context.Context, key string, value string, ttl time.Duration) error
    Del(ctx context.Context, keys ...string) error
}

// JWT配置接口
type JWTConfig interface {
    GetSecret() []byte
}
```

### 2. 服务层 (`api/user.go`)

```go
type UserService struct {
    db     repository.UserDB
    cache  repository.UserCache
    jwtCfg repository.JWTConfig
    jwtKey []byte
    ctx    context.Context
}

// NewUserService 构造函数，依赖注入
func NewUserService(db repository.UserDB, cache repository.UserCache, 
                   jwtCfg repository.JWTConfig, jwtKey []byte, ctx context.Context) *UserService {
    return &UserService{
        db:     db,
        cache:  cache,
        jwtCfg: jwtCfg,
        jwtKey: jwtKey,
        ctx:    ctx,
    }
}
```

## 完整的依赖注入实现示例

### 1. 修复后的 UserService 方法

#### AddUser 方法
```go
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
```

#### GetUserList 方法
```go
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
```

### 2. 实现具体的数据库和缓存实现

#### MySQL 实现 (`internal/repository/mysql_user.go`)
```go
package repository

import (
    "go-admin/model"
    "gorm.io/gorm"
)

type MySQLUserDB struct {
    db *gorm.DB
}

func NewMySQLUserDB(db *gorm.DB) *MySQLUserDB {
    return &MySQLUserDB{db: db}
}

func (m *MySQLUserDB) Create(user *model.User) error {
    return m.db.Create(user).Error
}

func (m *MySQLUserDB) FindByID(id uint) (*model.User, error) {
    var user model.User
    err := m.db.First(&user, id).Error
    return &user, err
}

func (m *MySQLUserDB) FindByUsername(username string) (*model.User, error) {
    var user model.User
    err := m.db.Where("username = ?", username).First(&user).Error
    return &user, err
}

func (m *MySQLUserDB) FindAll() ([]model.User, error) {
    var users []model.User
    err := m.db.Find(&users).Error
    return users, err
}

func (m *MySQLUserDB) CountByUsername(username string) (int64, error) {
    var count int64
    err := m.db.Model(&model.User{}).Where("username = ?", username).Count(&count).Error
    return count, err
}
```

#### Redis 实现 (`internal/repository/redis_cache.go`)
```go
package repository

import (
    "context"
    "time"
    "github.com/go-redis/redis/v8"
)

type RedisUserCache struct {
    client *redis.Client
}

func NewRedisUserCache(client *redis.Client) *RedisUserCache {
    return &RedisUserCache{client: client}
}

func (r *RedisUserCache) Get(ctx context.Context, key string) (string, error) {
    return r.client.Get(ctx, key).Result()
}

func (r *RedisUserCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
    return r.client.Set(ctx, key, value, ttl).Err()
}

func (r *RedisUserCache) Del(ctx context.Context, keys ...string) error {
    return r.client.Del(ctx, keys...).Err()
}
```

### 3. 依赖注入容器配置

#### 依赖配置 (`internal/container/container.go`)
```go
package container

import (
    "context"
    "go-admin/api"
    "go-admin/internal/repository"
    "go-admin/model"
    "gorm.io/gorm"
    "github.com/go-redis/redis/v8"
)

type Container struct {
    UserService *api.UserService
    // 其他服务...
}

func NewContainer(db *gorm.DB, redisClient *redis.Client, jwtSecret []byte) *Container {
    ctx := context.Background()
    
    // 创建具体实现
    userDB := repository.NewMySQLUserDB(db)
    userCache := repository.NewRedisUserCache(redisClient)
    
    // 创建JWT配置
    jwtCfg := repository.NewJWTConfig(jwtSecret)
    
    // 注入依赖
    userService := api.NewUserService(userDB, userCache, jwtCfg, jwtSecret, ctx)
    
    return &Container{
        UserService: userService,
    }
}
```

### 4. 路由配置中使用依赖注入

#### 修改路由 (`router/router.go`)
```go
package router

import (
    "go-admin/api"
    "go-admin/internal/container"
    "go-admin/middleware"
    "github.com/gin-gonic/gin"
)

func InitRouter(container *container.Container) *gin.Engine {
    r := gin.Default()
    r.Use(middleware.Cors())

    // 公开接口
    r.POST("/user/register", container.UserService.Register)
    r.POST("/user/login", container.UserService.Login)
    
    // 认证接口
    auth := r.Group("/auth")
    auth.Use(middleware.JWTAuth())
    {
        auth.GET("/user/list", container.UserService.GetUserList)
        auth.POST("/user/add", container.UserService.AddUser)
        // 其他接口...
    }
    
    return r
}
```

### 5. 主函数中使用依赖注入

#### 修改主函数 (`main.go`)
```go
package main

import (
    "go-admin/config"
    "go-admin/core"
    "go-admin/internal/container"
    "go-admin/model"
    "go-admin/router"
)

func main() {
    config.InitConfig()
    core.InitMysql()
    core.InitRedis()
    
    // 自动迁移
    core.DB.AutoMigrate(&model.User{})
    // 其他表...
    
    // 创建依赖注入容器
    container := container.NewContainer(core.DB, core.RDB, []byte(config.Config.Jwt.Secret))
    
    // 初始化路由
    r := router.InitRouter(container)
    r.Run(":" + config.Config.Server.Port)
}
```

## 依赖注入的优势体现

### 1. 易于测试
```go
// 测试用例
func TestUserService_AddUser(t *testing.T) {
    // 创建模拟的依赖
    mockDB := &MockUserDB{}
    mockCache := &MockUserCache{}
    
    // 注入模拟依赖
    service := api.NewUserService(mockDB, mockCache, mockJWT, []byte("secret"), context.Background())
    
    // 测试逻辑...
}
```

### 2. 易于扩展
```go
// 添加新的数据库实现
type MongoDBUserDB struct {
    client *mongo.Client
}

func NewMongoDBUserDB(client *mongo.Client) *MongoDBUserDB {
    return &MongoDBUserDB{client: client}
}

// 只需修改容器配置，无需修改业务逻辑
```

### 3. 配置灵活
```go
// 根据环境选择不同的实现
func NewContainer(env string, db *gorm.DB, redisClient *redis.Client) *Container {
    if env == "test" {
        userDB := repository.NewMockUserDB()
        userCache := repository.NewMockUserCache()
    } else {
        userDB := repository.NewMySQLUserDB(db)
        userCache := repository.NewRedisUserCache(redisClient)
    }
    // ...
}
```

## 下一步学习建议

1. **完善其他API的依赖注入**：按照这个模式改造帖子、评论等模块
2. **添加接口实现**：完成MySQL和Redis的具体实现
3. **添加单元测试**：为每个服务编写测试用例
4. **学习依赖注入框架**：如Google Wire、Uber Dig等
5. **微服务拆分准备**：依赖注入是微服务架构的基础

这个示例展示了如何将你的单体项目逐步改造为依赖注入架构，为后续的微服务拆分打下坚实基础。