# go-admin

基于 Go 构建的社交类后端服务，当前采用模块化单体架构，而不是微服务。

## 项目定位

当前项目已经具备这些核心能力：

- 用户注册、登录、资料查询、在线状态
- 帖子发布、列表、详情、个人帖子、点赞/收藏帖子查询
- 评论创建、修改、删除、帖子评论列表
- 点赞、点踩、收藏、分享、互动状态统计
- 关注、粉丝、拉黑、关系查询
- 私信会话、消息列表、发送消息、删除消息
- WebSocket 实时通信入口

## 当前架构

项目已经从原先较重的 `api` 风格，整理成下面这套分层结构：

```text
router -> handler -> service -> dao -> domain model
```

- 新功能可以沿固定路径落地，不容易写乱
- 业务逻辑集中在 `service`
- 数据访问集中在 `dao`
- 未来如果要拆微服务，也可以按模块逐步抽离

## 目录结构

```text
.
├─ cmd
│  └─ server
│     └─ main.go
├─ config
│  ├─ config.go
│  └─ config.yaml
├─ internal
│  ├─ container
│  │  └─ container.go
│  ├─ dao
│  │  ├─ comment_dao.go
│  │  ├─ interact_dao.go
│  │  ├─ jwt_config.go
│  │  ├─ message_dao.go
│  │  ├─ post_dao.go
│  │  ├─ post_scope.go
│  │  ├─ post_sort_strategy.go
│  │  ├─ social_dao.go
│  │  ├─ user_dao.go
│  │  ├─ user_gorm_dao.go
│  │  └─ user_redis_cache.go
│  ├─ domain
│  │  └─ model
│  │     ├─ comment.go
│  │     ├─ interact.go
│  │     ├─ message.go
│  │     ├─ post.go
│  │     ├─ social.go
│  │     └─ user.go
│  ├─ dto
│  │  ├─ comment_dto.go
│  │  ├─ message_dto.go
│  │  ├─ post_dto.go
│  │  ├─ user_dto.go
│  │  └─ ws_dto.go
│  ├─ handler
│  │  ├─ comment_handler.go
│  │  ├─ interact_handler.go
│  │  ├─ message_handler.go
│  │  ├─ post_handler.go
│  │  ├─ social_handler.go
│  │  ├─ user_handler.go
│  │  └─ ws_handler.go
│  ├─ middleware
│  │  ├─ admin.go
│  │  ├─ cors.go
│  │  ├─ custom_recovery.go
│  │  ├─ jwt.go
│  │  ├─ rate_limit.go
│  │  ├─ request_id.go
│  │  └─ request_logger.go
│  ├─ router
│  │  └─ router.go
│  └─ service
│     ├─ cache_support.go
│     ├─ comment_service.go
│     ├─ hash.go
│     ├─ interact_service.go
│     ├─ message_service.go
│     ├─ post_service.go
│     ├─ social_service.go
│     ├─ user_cache.go
│     ├─ user_service.go
│     ├─ ws_hub.go
│     └─ ws_service.go
├─ pkg
│  ├─ core
│  │  ├─ bind.go
│  │  ├─ logger.go
│  │  ├─ migrate.go
│  │  ├─ mysql.go
│  │  ├─ redis.go
│  │  ├─ response.go
│  │  └─ validator.go
│  └─ utils
│     └─ snowflake.go
├─ TEST
│  └─ test.md
├─ NEW_FEATURE_TEMPLATE.md
├─ a.exe
├─ go.mod
├─ go.sum
└─ README.md
```

## 分层职责

### `cmd/server`

程序启动入口，负责：

- 加载配置
- 初始化 MySQL、Redis
- 执行数据库迁移
- 初始化依赖注入容器
- 注册路由并启动服务

入口文件：`cmd/server/main.go`

### `config`

配置定义与配置加载。

相关文件：

- `config/config.go`
- `config/config.yaml`

### `internal/router`

路由注册层，只负责：

- 定义 URL
- 组织路由分组
- 绑定中间件
- 把请求交给对应 handler

这里不放业务逻辑，不直接操作数据库。

核心文件：`internal/router/router.go`

### `internal/handler`

请求处理层，负责：

- 接收 HTTP 或 WebSocket 请求
- 绑定参数到 DTO
- 做基础输入校验
- 调用 service
- 返回统一响应

这一层尽量薄，不要把业务规则写死在 handler 里。

### `internal/service`

业务逻辑层，是这套结构的核心。

负责：

- 编排业务流程
- 处理权限、状态、规则判断
- 调用 dao
- 处理缓存与跨模块逻辑

### `internal/dao`

数据访问层，负责：

- MySQL / Redis 的读写
- 对 Gorm、Redis 细节做封装
- 对 service 暴露稳定的数据访问接口

### `internal/dto`

接口输入输出对象。

原则：

- 前端传什么字段，就在 DTO 里定义什么字段
- DTO 不等于数据库模型
- 入参走 DTO，落库走 Model

### `internal/domain/model`

领域模型与数据库模型定义层。

负责：

- 定义 Gorm 模型
- 表达持久化结构
- 统一数据库字段结构

### `internal/middleware`

中间件层，当前包含：

- JWT 鉴权
- 管理员鉴权
- 限流
- 跨域
- 请求日志
- panic 恢复
- request id

### `pkg/core` 和 `pkg/utils`

放基础设施与通用工具，适合放：

- MySQL / Redis 初始化
- 自动迁移
- 响应封装
- 日志
- 校验器
- 通用工具函数

## 一次请求的流转过程

以“用户注册”为例：

1. `router` 注册 `POST /user/register`
2. 请求进入 `handler.UserHandler.Register`
3. handler 将 JSON 绑定到 `dto.RegisterRequest`
4. handler 调用 `service.UserService.Register`
5. service 做用户名校验、密码加密、默认角色赋值等业务处理
6. service 调用 `dao` 落库或写缓存
7. handler 使用 `pkg/core` 返回统一响应

## 启动流程

程序启动时会执行这些步骤：

1. 注册自定义校验器
2. 读取 `config/config.yaml`
3. 初始化 MySQL
4. 初始化 Redis
5. 执行自动迁移
6. 初始化容器 `internal/container`
7. 初始化路由
8. 启动 HTTP 服务

依赖注入文件：`internal/container/container.go`

## 路由概览

### 公开接口

- `POST /user/register`
- `POST /user/login`
- `GET /user/search`
- `GET /user/online/list`
- `GET /user/:id/online`
- `GET /user/:id`
- `GET /post/list`
- `GET /post/hot`
- `GET /post/:id`
- `GET /post/user/:id`
- `GET /post/user/:id/liked`
- `GET /post/user/:id/collected`
- `GET /comment/post/:post_id`
- `GET /interact/count/:post_id`
- `GET /ws`

### 需要登录的接口

请求头：

```text
Authorization: Bearer <token>
```

- `GET /auth/user/list`
- `GET /auth/user/:id`
- `POST /auth/user/logout`
- `POST /auth/user/batch-roles`
- `PUT /auth/user/:id`
- `PUT /auth/user/password/:id`
- `POST /auth/post/create`
- `GET /auth/post/my`
- `PUT /auth/post/:id`
- `DELETE /auth/post/:id`
- `POST /auth/comment/create`
- `DELETE /auth/comment/:id`
- `PUT /auth/comment/:id`
- `GET /auth/comment/my`
- `POST /auth/interact/like/:post_id`
- `POST /auth/interact/dislike/:post_id`
- `POST /auth/interact/collect/:post_id`
- `POST /auth/interact/share/:post_id`
- `GET /auth/interact/status/:post_id`
- `POST /auth/social/follow/:uid`
- `POST /auth/social/block/:uid`
- `GET /auth/social/relation/:uid`
- `GET /auth/social/follows`
- `GET /auth/social/followers`
- `GET /auth/social/blocks`
- `GET /auth/message/conversations`
- `GET /auth/message/list`
- `POST /auth/message/send`
- `DELETE /auth/message/:id`

### 管理员接口

- `POST /auth/user/add`
- `DELETE /auth/user/:id`

## 数据库迁移

启动时会自动迁移以下模型：

- User
- Post
- Comment
- Like
- Dislike
- Collect
- Share
- UserRelation
- Message

相关文件：`pkg/core/migrate.go`

## 本地运行

### 环境要求

- Go `1.25.8`
- MySQL
- Redis

### 安装依赖

```bash
go mod tidy
```

### 启动项目

默认命令均在仓库根目录执行：

```bash
go run ./cmd/server
```

默认监听地址：

```text
http://localhost:19999
```

## 构建可执行文件

构建 Windows 可执行文件：

```bash
go build -o a.exe ./cmd/server
```

如果只是检查是否可编译：

```bash
go build ./cmd/server
```

## 配置说明

默认配置文件：

- `config/config.yaml`

示例：

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
  port: 16379
  password: ""
  db: 0

jwt:
  secret: "replace-this-in-production"
```

## 开发约定

后续加功能时，建议固定按这个顺序落代码：

1. 先定义 `domain/model`
2. 再定义 `dto`
3. 再补 `dao`
4. 再补 `service`
5. 再补 `handler`
6. 最后注册 `router`

建议遵循这些原则：

- `handler` 不直接写数据库
- `dao` 不承担复杂业务规则
- `service` 不依赖 Gin 上下文
- DTO 和 Model 分开
- 公共基础能力放 `pkg/core` 或 `pkg/utils`

## 补充文档

- 新功能开发模板：`future.md`
- API 测试文档：`TEST/test.md`

