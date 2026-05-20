# go-admin

`go-admin` 是一个基于 Go 的社交/聊天后端项目。当前仍是模块化单体架构，但已经引入实时通信、群聊、RabbitMQ 广播、Redis 在线状态、MySQL 消息持久化、outbox 可靠发布、DLQ 和基础监控能力。

项目适合用来学习从普通 CRUD 后端逐步演进到高并发实时聊天系统的过程。

## 核心能力

- 用户注册、登录、资料查询、在线状态查询
- 帖子发布、列表、详情、个人帖子、点赞/收藏帖子查询
- 评论创建、修改、删除、帖子评论列表
- 点赞、点踩、收藏、分享、互动状态统计
- 关注、粉丝、拉黑、关系查询
- 单聊消息、会话列表、消息列表、消息删除
- 群创建、群成员管理、邀请、入群申请、退群、解散
- HTTP 群消息接口：发送群消息、查询群消息
- WebSocket 实时通信：单聊、群聊、心跳
- RabbitMQ fanout 群聊广播，每个实例独立队列
- Redis 在线状态、实例路由、群成员缓存、群在线成员缓存
- outbox 可靠事件发布，避免“消息入库成功但 MQ 发布失败”导致丢消息
- RabbitMQ DLQ，隔离 consumer 处理失败的坏消息
- QUIC Gateway 基础连接管理实验模块
- `/metrics` 基础监控指标和 pprof 性能分析入口

## 技术栈

- Go `1.25.8`
- Gin
- GORM
- MySQL
- Redis
- RabbitMQ
- Gorilla WebSocket
- quic-go

## 架构分层

当前主链路采用：

```text
router -> middleware -> handler -> service -> dao -> domain model
```

职责划分：

- `router`：注册 HTTP/WebSocket 路由。
- `middleware`：JWT、限流、CORS、日志、panic recovery、request id。
- `handler`：参数绑定、基础校验、调用 service、返回响应。
- `service`：业务编排、状态校验、缓存、MQ、实时推送。
- `dao`：MySQL/Redis 数据访问封装。
- `domain/model`：GORM 持久化模型。
- `dto`：接口入参、出参和事件结构。
- `pkg/core`：MySQL、Redis、RabbitMQ、日志、响应、迁移、监控等基础设施。

## 目录结构

```text
.
├── cmd/server              # 程序启动入口
├── config                  # 配置加载与配置文件
├── internal/container      # 依赖注入组装
├── internal/dao            # MySQL/Redis 数据访问
├── internal/domain/model   # GORM 模型
├── internal/dto            # 请求、响应、事件 DTO
├── internal/gateway        # QUIC Gateway 实验模块
├── internal/handler        # HTTP/WebSocket handler
├── internal/middleware     # Gin 中间件
├── internal/mq             # MQ consumer pool 实验模块
├── internal/router         # 路由注册
├── internal/service        # 业务逻辑
├── pkg/core                # 基础设施
├── pkg/utils               # 通用工具
├── plan.md                 # 高并发聊天系统设计计划
├── reliability.md          # 可靠性、压测、告警、灰度、演练手册
├── go.mod
└── README.md
```

## 启动流程

程序入口：[F:\DI\cmd\server\main.go](F:\DI\cmd\server\main.go)

启动时会执行：

1. 注册自定义参数校验器。
2. 加载 `config/config.yaml`。
3. 初始化 MySQL。
4. 初始化 Redis。
5. 异步执行 AutoMigrate。
6. 初始化依赖注入容器。
7. 初始化 RabbitMQ、WebSocket Hub、consumer、outbox publisher。
8. 注册 Gin 路由。
9. 启动 HTTP 服务。
10. 启动 pprof 和 `/metrics` 监听。

## 本地运行

### 依赖服务

需要先准备：

- MySQL
- Redis
- RabbitMQ

默认配置文件：[F:\DI\config\config.yaml](F:\DI\config\config.yaml)

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
  secret: "syloe1-change-this-in-production"

rabbitmq:
  url: "amqp://guest:guest@127.0.0.1:5672/"
  exchange: "chat.events"
  queue: "group_message_push"
```

### 安装依赖

```bash
go mod tidy
```

### 启动项目

```bash
go run ./cmd/server
```

默认 HTTP 服务地址：

```text
http://localhost:19999
```

pprof 和 metrics 地址：

```text
http://127.0.0.1:6060/debug/pprof
http://127.0.0.1:6060/metrics
```

### 编译检查

```bash
go build ./...
```

## 多实例部署说明

项目支持多实例消费群聊广播。每个实例会生成自己的实例 ID：

- 优先使用环境变量 `INSTANCE_ID`
- 如果没有配置，则使用 `hostname-pid`

RabbitMQ 队列格式：

```text
<rabbitmq.queue>.<instance_id>
```

例如：

```text
group_message_push.chat-1
group_message_push.chat-2
```

每个实例队列都会绑定到同一个 fanout exchange：

```text
chat.events
```

这样一条群消息事件会广播到每个实例，每个实例只推送自己本机在线的用户。

## 群消息链路

### HTTP 发送群消息

```text
POST /auth/groups/:id/messages
Authorization: Bearer <token>
Content-Type: application/json
```

请求示例：

```json
{
  "content": "大家好，这是一条 HTTP 群消息"
}
```

处理流程：

```text
handler
  -> service 校验用户是 active 成员、群状态 normal
  -> MySQL 写入 chat_group_messages
  -> 同事务写入 message_outboxes
  -> OutboxService 后台发布 RabbitMQ 事件
  -> 每个实例 consumer 收到事件
  -> 查询本机在线群成员
  -> WebSocket 推送
```

### HTTP 查询群消息

```text
GET /auth/groups/:id/messages?page=1&pageSize=20
Authorization: Bearer <token>
```

### WebSocket 发送群消息

连接地址：

```text
ws://localhost:19999/ws?token=<JWT>
```

发送：

```json
{
  "type": "group_message",
  "group_id": 1,
  "content": "大家好，这是一条 WebSocket 群消息"
}
```

群成员收到：

```json
{
  "type": "group_message",
  "message_id": 12,
  "group_id": 1,
  "from_uid": 3,
  "content": "大家好，这是一条 WebSocket 群消息",
  "time": "2026-05-16T14:20:00+08:00"
}
```

### WebSocket 心跳

发送：

```json
{
  "type": "ping"
}
```

收到：

```json
{
  "type": "pong",
  "time": "2026-05-20T10:00:00+08:00"
}
```

## RabbitMQ 与可靠性

### Producer/Consumer channel 分离

RabbitMQ 初始化后会创建：

- `PublishChannel`：只用于发布消息。
- `ConsumeChannel`：只用于消费消息。

这样可以避免发布和消费互相影响。

### Outbox 可靠发布

群消息落库时，不直接依赖 MQ 发布成功，而是先把事件写入 `message_outboxes`。

关键状态：

- `pending`：等待发布。
- `published`：已发布成功。
- `failed`：重试多次仍失败。

`OutboxService` 会定时扫描到期事件并发布 MQ，失败后按退避时间重试。

### DLQ

每个实例队列都有对应死信队列：

```text
<queue>.<instance_id>.dlq
```

consumer 处理失败时会 `Nack(false, false)`，消息进入 DLQ，避免坏消息无限重试拖垮 worker pool。

## Redis 缓存

当前 Redis 主要用于：

- 用户在线状态
- 实例 route
- 群 active 成员缓存
- 群在线成员缓存
- 大群在线成员分片

群消息推送时优先使用 Redis 缓存拿到在线成员分片，再通过 worker pool 并行推送。

## 慢连接降级

每个 WebSocket 连接有独立 `Send` 队列。推送时如果队列已满，说明客户端读取过慢，服务端会主动断开该连接。

这样可以避免一个慢连接拖慢整个群推送。

## QUIC Gateway

QUIC 代码位于：[F:\DI\internal\gateway](F:\DI\internal\gateway)

当前是最小闭环实验版本：

```text
QUIC 连接 -> 首个 stream 认证 uid -> 注册 session -> Enqueue 推送 -> writeLoop 写回 -> Close 自动注销
```

当前能力：

- 单实例本地 session 管理
- 新连接顶掉旧连接
- send queue
- `PING`/`PONG`
- 连接超时关闭
- 可选 Redis presence

当前还没有把主业务入口完全切到 QUIC，WebSocket 仍是主要实时通信入口。

## 监控指标

访问：

```text
http://127.0.0.1:6060/metrics
```

当前指标：

```text
chat_online_connections
chat_push_delivered_total
chat_push_dropped_slow_total
chat_consumer_succeeded_total
chat_consumer_failed_total
chat_outbox_published_total
chat_outbox_publish_failed_total
```

配合 [F:\DI\reliability.md](F:\DI\reliability.md) 可以进行压测、容量评估、告警配置、灰度发布和故障演练。

## 路由概览

### 公开接口

- `POST /user/register`
- `POST /user/login`
- `GET /post/list`
- `GET /post/hot`
- `GET /post/:id`
- `GET /post/user/:id`
- `GET /post/user/:id/liked`
- `GET /post/user/:id/collected`
- `GET /comment/post/:post_id`
- `GET /interact/count/:post_id`
- `GET /user/search`
- `GET /user/online/list`
- `GET /user/:id/online`
- `GET /user/:id`
- `GET /ws`

### 登录接口

请求头：

```text
Authorization: Bearer <token>
```

用户：

- `GET /auth/user/list`
- `GET /auth/user/:id`
- `POST /auth/user/logout`
- `POST /auth/user/batch-roles`
- `PUT /auth/user/:id`
- `PUT /auth/user/password/:id`

帖子：

- `POST /auth/post/create`
- `GET /auth/post/my`
- `PUT /auth/post/:id`
- `DELETE /auth/post/:id`

评论：

- `POST /auth/comment/create`
- `DELETE /auth/comment/:id`
- `PUT /auth/comment/:id`
- `GET /auth/comment/my`

互动：

- `POST /auth/interact/like/:post_id`
- `POST /auth/interact/dislike/:post_id`
- `POST /auth/interact/collect/:post_id`
- `POST /auth/interact/share/:post_id`
- `GET /auth/interact/status/:post_id`

社交：

- `POST /auth/social/follow/:uid`
- `POST /auth/social/block/:uid`
- `GET /auth/social/relation/:uid`
- `GET /auth/social/follows`
- `GET /auth/social/followers`
- `GET /auth/social/blocks`

单聊：

- `GET /auth/message/conversations`
- `GET /auth/message/list`
- `POST /auth/message/send`
- `DELETE /auth/message/:id`

群聊：

- `POST /auth/groups`
- `GET /auth/groups/my`
- `GET /auth/groups/:id`
- `GET /auth/groups/:id/members`
- `GET /auth/groups/:id/join-requests`
- `POST /auth/groups/:id/admins`
- `DELETE /auth/groups/:id/admins/:user_id`
- `DELETE /auth/groups/:id/members/:user_id`
- `POST /auth/groups/:id/transfer-owner`
- `POST /auth/groups/:id/dissolve`
- `POST /auth/groups/:id/invitations`
- `POST /auth/groups/invitations/:id/review`
- `POST /auth/groups/:id/join-requests`
- `POST /auth/groups/join-requests/:id/review`
- `POST /auth/groups/:id/leave`
- `POST /auth/groups/:id/messages`
- `GET /auth/groups/:id/messages`

管理员：

- `POST /auth/user/add`
- `DELETE /auth/user/:id`

## 数据库迁移

启动时会自动迁移：

- `User`
- `Post`
- `Comment`
- `Like`
- `Dislike`
- `Collect`
- `Share`
- `UserRelation`
- `Message`
- `ChatGroup`
- `ChatGroupMember`
- `ChatGroupInvitation`
- `ChatGroupJoinRequest`
- `ChatGroupMessage`
- `MessageOutbox`

迁移入口：[F:\DI\pkg\core\migrate.go](F:\DI\pkg\core\migrate.go)

## 开发约定

新增业务建议按这个顺序：

1. 定义 `domain/model`
2. 定义 `dto`
3. 补充 `dao`
4. 编写 `service`
5. 编写 `handler`
6. 注册 `router`
7. 必要时补充 MQ、Redis、metrics 和文档

约定：

- `handler` 不直接写数据库。
- `dao` 不承载复杂业务规则。
- `service` 不依赖 Gin 上下文。
- DTO 和 Model 分开。
- 需要事务一致性的地方放在 DAO 层统一处理。
- 可靠事件发布优先使用 outbox，不在业务主流程里直接依赖 MQ 成功。

## 相关文档

- [F:\DI\plan.md](F:\DI\plan.md)：生产级高并发聊天系统设计计划。
- [F:\DI\reliability.md](F:\DI\reliability.md)：可靠性、压测、告警、灰度和故障演练。
- [F:\DI\future.md](F:\DI\future.md)：后续功能规划。
