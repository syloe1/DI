# Go-Admin 社交平台后端

一个基于 `Gin + GORM + MySQL + Redis` 的社交平台后端示例项目，支持用户注册登录、帖子发布、评论互动、关注拉黑、私信聊天和 WebSocket 实时消息。

这版项目已经按“配置 -> 核心连接 -> 数据模型 -> 仓储 -> API -> 路由”的方式做了分层整理，并保留手写 `Container` 的依赖注入模式，不使用 Wire、不拆微服务。

## 技术栈

- Go
- Gin
- GORM
- MySQL
- Redis
- JWT
- WebSocket

## 主要功能

- 用户注册、登录、搜索、资料查询
- 角色权限控制：`user / admin / superadmin`
- 帖子创建、编辑、删除、公开列表、用户帖子列表
- 评论创建、删除、修改、查询
- 点赞、点踩、收藏、分享、互动状态查询
- 关注、取关、拉黑、取消拉黑、关系状态查询
- 私信发送、会话列表、消息列表、已读处理
- WebSocket 实时消息

## 当前架构

项目遵循这条依赖方向：

`config -> core -> model -> repository -> api -> router -> main`

说明：

- `config`：只负责读取配置，不保存全局单例状态
- `core`：负责 MySQL、Redis、日志、统一响应、自动迁移
- `model`：定义数据库模型
- `repository`：封装数据库/缓存访问接口与实现
- `api`：处理业务逻辑和 HTTP 请求
- `router`：只做路由注册和中间件挂载
- `main`：统一装配依赖，创建容器并启动服务

## DI 说明

项目保留手写容器模式，在 [container.go](</F:/go/internal/container/container.go:1>) 中集中创建依赖：

- 配置 `config.App`
- MySQL `*gorm.DB`
- Redis `*redis.Client`
- JWT Secret
- Repository 实现
- API Handler 实例

这样做的好处：

- 依赖来源清晰，入口集中
- 减少全局变量耦合
- 更方便做单元测试和替换 mock
- 对新手也比较直观，容易顺着调用链看懂

## 项目结构

```text
F:\go
├── api
│   ├── user.go
│   ├── post.go
│   ├── comment_api.go
│   ├── interact_api.go
│   ├── social_api.go
│   ├── message_api.go
│   └── ws_api.go
├── config
│   ├── config.go
│   └── config.yaml
├── core
│   ├── mysql.go
│   ├── redis.go
│   ├── response.go
│   ├── logger.go
│   └── migrate.go
├── internal
│   ├── container
│   │   └── container.go
│   └── repository
│       ├── user_repo.go
│       ├── gorm_user.go
│       ├── redis_user.go
│       ├── post_repository.go
│       ├── comment_repository.go
│       ├── interact_repository.go
│       ├── social_repository.go
│       ├── message_repository.go
│       └── jwt_config.go
├── middleware
│   ├── cors.go
│   ├── jwt.go
│   └── admin.go
├── model
├── router
│   └── di_router.go
├── main.go
└── README.md
```

## 启动流程

启动入口在 [main.go](</F:/go/main.go:1>)，调用链如下：

1. `config.Load("config/config.yaml")` 读取配置
2. `core.InitMysql(...)` 初始化 MySQL
3. `core.InitRedis(...)` 初始化 Redis
4. `core.AutoMigrate(db)` 自动迁移表结构
5. `container.NewContainer(...)` 创建依赖注入容器
6. `router.InitDependencyInjectionRouter(...)` 注册路由
7. `r.Run(...)` 启动 Gin 服务

## 运行要求

- Go 1.20+
- MySQL 5.7+
- Redis 5.0+

## 配置示例

配置文件位置：[config.yaml](</F:/go/config/config.yaml:1>)

```yaml
server:
  port: 19999

mysql:
  host: 127.0.0.1
  port: 3307
  dbname: go
  username: root
  password: 123456

redis:
  host: 127.0.0.1
  port: 6379
  password: ""
  db: 0

jwt:
  secret: "syloe1-change-this-in-production"
```

## 本地运行

安装依赖：

```bash
go mod tidy
```

直接运行：

```bash
go run main.go
```

按当前项目习惯编译：

```cmd
go build -o a.exe
a.exe
```

默认启动地址：

```text
http://localhost:19999
```

## 统一响应结构

公共响应封装在 [response.go](</F:/go/core/response.go:1>)：

```json
{
  "code": 200,
  "msg": "success",
  "data": {}
}
```

失败时同样保持统一结构：

```json
{
  "code": 400,
  "msg": "参数错误"
}
```

## 路由说明

路由集中在 [di_router.go](</F:/go/router/di_router.go:1>)。

公开接口示例：

- `POST /user/register`
- `POST /user/login`
- `GET /post/list`
- `GET /post/:id`
- `GET /user/search`
- `GET /ws`

登录后接口示例：

- `POST /auth/post/create`
- `PUT /auth/post/:id`
- `POST /auth/comment/create`
- `POST /auth/interact/like/:post_id`
- `POST /auth/social/follow/:uid`
- `POST /auth/message/send`

管理员接口示例：

- `POST /auth/user/add`
- `DELETE /auth/user/:id`

## 测试方式

你可以继续使用 Postman + JSON 测试接口，这也是当前项目最直接的联调方式。

推荐测试顺序：

1. 注册用户
2. 登录获取 JWT
3. 携带 `Authorization: Bearer <token>` 测试 `/auth/*` 接口
4. 再测试关注、互动、消息等链路

接口测试说明可以参考 [test.md](</F:/go/test.md:1>)。

## 接口调用示例

下面这几组示例可以直接用于 Postman。

### 1. 用户注册

请求：

```http
POST /user/register
Content-Type: application/json
```

```json
{
  "username": "test_user_01",
  "password": "123456"
}
```

### 2. 用户登录

请求：

```http
POST /user/login
Content-Type: application/json
```

```json
{
  "username": "test_user_01",
  "password": "123456"
}
```

返回成功后，从响应里取出：

```json
{
  "code": 200,
  "msg": "success",
  "data": {
    "token": "your-jwt-token",
    "role": "user"
  }
}
```

后续访问 `/auth/*` 接口时，请在请求头中带上：

```http
Authorization: Bearer your-jwt-token
```

### 3. 创建帖子

请求：

```http
POST /auth/post/create
Authorization: Bearer your-jwt-token
Content-Type: application/json
```

```json
{
  "title": "我的第一篇帖子",
  "content": "大家好，这是一个 #Gin #GORM 测试帖子",
  "is_public": true,
  "status": "published",
  "topics": "后端,Go",
  "images": ""
}
```

### 4. 获取帖子列表

请求：

```http
GET /post/list
```

按话题筛选：

```http
GET /post/list?topic=Gin
```

### 5. 获取单个帖子

请求：

```http
GET /post/1
```

### 6. 修改帖子

请求：

```http
PUT /auth/post/1
Authorization: Bearer your-jwt-token
Content-Type: application/json
```

```json
{
  "title": "修改后的帖子标题",
  "content": "这里是修改后的内容 #更新",
  "is_public": true,
  "topics": "更新,Go"
}
```

### 7. 创建评论

请求：

```http
POST /auth/comment/create
Authorization: Bearer your-jwt-token
Content-Type: application/json
```

```json
{
  "post_id": 1,
  "content": "这是我的评论内容",
  "parent_id": 0
}
```

### 8. 获取某个帖子的评论

请求：

```http
GET /comment/post/1
```

### 9. 点赞帖子

请求：

```http
POST /auth/interact/like/1
Authorization: Bearer your-jwt-token
```

说明：

- 再次调用同一接口，通常会变成取消点赞
- 点赞和点踩在当前逻辑里是互斥的

### 10. 点踩帖子

请求：

```http
POST /auth/interact/dislike/1
Authorization: Bearer your-jwt-token
```

### 11. 收藏帖子

请求：

```http
POST /auth/interact/collect/1
Authorization: Bearer your-jwt-token
```

### 12. 查询互动状态

请求：

```http
GET /auth/interact/status/1
Authorization: Bearer your-jwt-token
```

### 13. 关注用户

请求：

```http
POST /auth/social/follow/2
Authorization: Bearer your-jwt-token
```

说明：

- `2` 是目标用户 ID
- 再次调用通常会变成取消关注

### 14. 拉黑用户

请求：

```http
POST /auth/social/block/2
Authorization: Bearer your-jwt-token
```

### 15. 发送私信

请求：

```http
POST /auth/message/send
Authorization: Bearer your-jwt-token
Content-Type: application/json
```

```json
{
  "to_uid": 2,
  "content": "你好，这是一条测试私信"
}
```

### 16. 获取会话列表

请求：

```http
GET /auth/message/conversations
Authorization: Bearer your-jwt-token
```

### 17. 获取和某个用户的消息列表

请求：

```http
GET /auth/message/list?peer_id=2&limit=20
Authorization: Bearer your-jwt-token
```

### 18. 搜索用户

请求：

```http
GET /user/search?username=test
```

### 19. 获取用户信息

请求：

```http
GET /user/1
```

### 20. WebSocket 连接示例

连接地址：

```text
ws://localhost:19999/ws?token=your-jwt-token
```

发送消息示例：

```json
{
  "type": "message",
  "to_uid": 2,
  "content": "这是一条 websocket 消息"
}
```

心跳包示例：

```json
{
  "type": "ping"
}
```

## Postman 使用建议

为了方便联调，建议在 Postman 里这样设置：

1. 创建环境变量 `base_url`
2. 设置 `base_url = http://localhost:19999`
3. 登录成功后把 `token` 保存成环境变量
4. 在需要登录的接口里统一使用：

```http
Authorization: Bearer {{token}}
```

这样后面测试 `/auth/*` 接口会省很多事。

## 管理员接口示例

下面这组示例适合 `admin` 或 `superadmin` 账号测试。

注意：

- 普通用户访问管理员接口会返回无权限
- 建议先用管理员账号登录，再把返回的 token 放进 Postman 环境变量

### 1. 获取用户列表

请求：

```http
GET /auth/user/list
Authorization: Bearer {{token}}
```

### 2. 根据 ID 获取用户信息

请求：

```http
GET /auth/user/1
Authorization: Bearer {{token}}
```

### 3. 批量查询用户角色

请求：

```http
POST /auth/user/batch-roles
Authorization: Bearer {{token}}
Content-Type: application/json
```

```json
{
  "ids": [1, 2, 3]
}
```

### 4. 管理员添加用户

请求：

```http
POST /auth/user/add
Authorization: Bearer {{token}}
Content-Type: application/json
```

```json
{
  "username": "admin_created_user",
  "password": "123456",
  "role": "user"
}
```

说明：

- 如果不传 `role`，当前逻辑通常会默认创建为 `user`
- 这个接口适合管理员批量创建测试账号

### 5. 修改用户信息

请求：

```http
PUT /auth/user/2
Authorization: Bearer {{token}}
Content-Type: application/json
```

```json
{
  "username": "updated_name",
  "role": "user"
}
```

说明：

- 超级管理员可以改更多角色字段
- 管理员通常只能把目标用户角色改成 `user`

### 6. 修改用户密码

请求：

```http
PUT /auth/user/password/2
Authorization: Bearer {{token}}
Content-Type: application/json
```

```json
{
  "oldPassword": "123456",
  "newPassword": "654321"
}
```

说明：

- 普通用户修改自己密码时，一般需要提供正确旧密码
- 管理员和超级管理员改别人密码时，逻辑限制以当前代码实现为准

### 7. 删除用户

请求：

```http
DELETE /auth/user/2
Authorization: Bearer {{token}}
```

### 8. 管理员接口测试建议顺序

推荐顺序：

1. 管理员登录
2. 获取用户列表，确认已有用户 ID
3. 调用 `/auth/user/add` 新建测试用户
4. 调用 `/auth/user/2` 或实际 ID 查询用户
5. 调用 `/auth/user/batch-roles` 验证角色批量查询
6. 调用 `/auth/user/password/:id` 修改密码
7. 最后调用 `DELETE /auth/user/:id` 清理测试数据

## 适合继续优化的方向

在不改变当前单体架构和手写 DI 风格的前提下，后续可以继续做：

- 给多步写操作补事务
- 增加 repository/service 的 mock 测试
- 补统一错误码常量
- 补 Swagger 或更完整的接口文档
- 优化日志分级
- 收敛部分重复的参数校验与响应文案

## 说明

这个项目现在更适合作为：

- Gin + GORM 分层练手项目
- 手写依赖注入示例项目
- 社交类后端接口练习项目
- 后续继续补事务和单元测试的基础工程
