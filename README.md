# go-admin

基于 Go 构建的社交类后端服务，当前采用“模块化单体”架构，而不是微服务。

这套结构的目标不是为了把项目拆得很花，而是先把职责边界理顺，让你后续继续加功能、排查问题、补测试都更轻松。如果未来某个模块真的长大了，再从现在的单体里抽成独立服务会更稳。

## 项目定位

项目当前提供的核心能力包括：

- 用户注册、登录、资料查询、在线状态
- 帖子发布、列表、详情、个人帖子、点赞/收藏帖子查询
- 评论创建、修改、删除、帖子评论列表
- 点赞、点踩、收藏、分享、互动状态统计
- 关注、粉丝、拉黑、关系查询
- 私信会话、消息列表、发送消息、删除消息
- WebSocket 实时通信入口

## 当前架构

当前项目已经从原先较重的 `api` 层，整理为清晰的分层结构：

```text
router -> handler -> service -> dao -> domain model
```

这是一种适合中小型项目持续演进的模块化单体架构。

优点：

- 比原来的“大而全 api 文件”更容易维护
- 新功能可以沿着固定路径落地，不容易写乱
- 业务逻辑集中在 `service`，不会散落到路由和数据库层
- 未来如果要拆微服务，可以按模块逐步抽离，而不是现在就把复杂度一次性拉满

## 目录结构

```text
F:\go
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
├─ a.exe
├─ go.mod
├─ go.sum
└─ README.md
```

## 分层职责

### 1. `cmd/server`

程序启动入口。

负责：

- 加载配置
- 初始化 MySQL、Redis
- 执行数据库迁移
- 初始化依赖注入容器
- 注册路由并启动 Gin 服务

入口文件：[`F:\go\cmd\server\main.go`](/F:/go/cmd/server/main.go)

### 2. `config`

配置读取与配置结构定义。

当前配置文件仍然保留在 `config` 目录下，这一层不需要为了分层而强行拆走。

相关文件：

- [`F:\go\config\config.go`](/F:/go/config/config.go)
- [`F:\go\config\config.yaml`](/F:/go/config/config.yaml)

### 3. `internal/router`

路由注册层。

职责只包括：

- 定义 URL
- 组织路由分组
- 绑定中间件
- 将请求交给对应 handler

这里不放业务逻辑，不直接操作数据库。

相关文件：[`F:\go\internal\router\router.go`](/F:/go/internal/router/router.go)

### 4. `internal/handler`

请求处理层。

职责包括：

- 接收 HTTP 或 WebSocket 请求
- 绑定参数到 DTO
- 做基础输入校验
- 调用 service
- 将 service 返回结果转成统一响应

这一层要尽量薄，避免把业务规则写死在 handler 里。

### 5. `internal/service`

业务逻辑层，是这套架构的核心。

职责包括：

- 编排业务流程
- 处理权限、状态、规则判断
- 调用 dao 持久化数据
- 调用缓存能力
- 组织跨模块逻辑

如果后面要补单元测试，`service` 会是最值得优先覆盖的一层。

### 6. `internal/dao`

数据访问层。

职责包括：

- 面向数据库和缓存的读写
- 对 Gorm、Redis 细节做封装
- 为 service 暴露稳定的数据访问接口

这里尽量不写业务判断，重点是“查什么、存什么、怎么查”。

### 7. `internal/dto`

请求和响应的数据传输对象。

职责包括：

- 定义接口入参
- 定义部分响应结构
- 与 handler 的绑定逻辑配合

注意：

- DTO 不等于数据库模型
- 前端传什么字段，就在 DTO 里定义什么字段
- 数据库需要什么字段，由 `domain/model` 决定

例如注册接口中，前端不会传 `role`，所以 `RegisterRequest` 不需要 `role`；但创建用户落库时，service 可以给模型补上默认值 `user`

### 8. `internal/domain/model`

领域模型与数据库模型定义。

职责包括：

- 定义 Gorm 模型
- 表达持久化结构
- 表达领域中的核心对象

这一层是数据库结构的中心，不建议直接拿来当所有接口的入参结构。

### 9. `internal/middleware`

中间件层。

当前包含：

- JWT 鉴权
- 管理员鉴权
- 限流
- 跨域
- 请求日志
- panic 恢复
- request id

### 10. `pkg/core` 和 `pkg/utils`

放基础设施与通用工具。

适合放：

- MySQL 初始化
- Redis 初始化
- 响应封装
- 日志
- 自动迁移
- 校验器注册
- 雪花算法等通用能力

不适合放具体业务逻辑。

## 一次请求的流转过程

以“用户注册”为例：

1. `router` 注册 `POST /user/register`
2. 请求进入 `handler.UserHandler.Register`
3. handler 将 JSON 绑定到 `dto.RegisterRequest`
4. handler 调用 `service.UserService.Register`
5. service 做用户名校验、密码加密、默认角色赋值、缓存处理
6. service 调用 `dao` 写入 MySQL / Redis
7. handler 使用 `pkg/core` 返回统一响应

这条链路的价值在于：

- 路由层简单
- handler 只管收发请求
- 业务规则集中在 service
- 数据操作集中在 dao

## 启动流程

程序启动时会执行以下步骤：

1. 注册自定义校验器
2. 读取 `config/config.yaml`
3. 初始化 MySQL
4. 初始化 Redis
5. 自动执行数据库迁移
6. 初始化容器 `internal/container`
7. 初始化路由
8. 启动 HTTP 服务

依赖注入容器文件：[`F:\go\internal\container\container.go`](/F:/go/internal/container/container.go)

## 路由分组概览

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

管理员接口是在登录基础上叠加管理员中间件：

- `POST /auth/user/add`
- `DELETE /auth/user/:id`

## 数据库迁移

程序启动时会自动迁移以下模型：

- User
- Post
- Comment
- Like
- Dislike
- Collect
- Share
- UserRelation
- Message

相关文件：[`F:\go\pkg\core\migrate.go`](/F:/go/pkg/core/migrate.go)

## 本地运行

### 环境要求

- Go `1.25.8`
- MySQL
- Redis

### 安装依赖

```powershell
go mod tidy
```

### 启动项目

```powershell
go run .\cmd\server
```

默认监听地址：

```text
http://localhost:19999
```

## 构建可执行文件

生成 Windows 可执行文件：

```powershell
go build -o .\a.exe .\cmd\server
```

当前仓库根目录已经存在一个已编译文件：

- [`F:\go\a.exe`](/F:/go/a.exe)

如果只是检查是否可编译：

```powershell
go build .\cmd\server
```

## 配置说明

默认配置文件：

- [`F:\go\config\config.yaml`](/F:/go/config/config.yaml)

示例结构：

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

## 这套结构下的开发约定

后续继续加功能时，建议固定按下面的顺序落代码：

1. 先定义 `domain/model`，明确数据结构
2. 定义 `dto`，明确接口收什么、回什么
3. 在 `dao` 中补数据访问能力
4. 在 `service` 中补业务逻辑
5. 在 `handler` 中补请求接入
6. 在 `router` 中注册路由

建议遵循这些原则：

- `handler` 不直接写数据库
- `dao` 不承担复杂业务规则
- `service` 不依赖 Gin 上下文
- DTO 和 Model 分开，不混用
- 公共基础能力放 `pkg/core` 或 `pkg/utils`
- 模块内部优先高内聚，先把单体做完整，再考虑拆服务

## 关于是否要拆微服务

基于你当前这个项目阶段，更推荐继续保持现在这套模块化单体结构，而不是立刻拆微服务。

原因很实际：

- 业务边界还在变化，过早拆分会放大沟通成本和调用复杂度
- gRPC、服务注册、链路追踪、分布式事务、部署编排都会带来额外负担
- 如果单体本身还没稳定，拆出去的服务边界通常也不稳定，后面还会反复重构

更合理的节奏是：

1. 先把单体结构整理干净
2. 先把功能补完整、职责边界跑顺
3. 观察哪些模块真的独立、调用频繁、变更节奏不同
4. 再决定是否把某个模块抽成独立服务

也就是说，你现在这一步改成 `handler / service / dao / dto / domain`，方向是对的，而且比“为了微服务而微服务”更稳。

## 当前状态

目前项目已经完成从旧 `api` 风格向分层结构的迁移，适合作为后续继续开发和补文档的基础版本。

如果后面你愿意，我们下一步可以继续做这三件事中的任意一个：

1. 给每个模块补一份更细的接口说明
2. 生成 Swagger / OpenAPI 文档
3. 继续把测试结构也整理成统一规范
