# DI — 高性能实时 IM 后端

`DI` 是一个基于 Go 的**实时即时通讯（IM）后端**：一对一私聊 + 群组聊天，模块化单体架构。围绕"消息不丢、多实例可扩、在线状态一致"三条主线，接入 WebSocket 长连接、RabbitMQ 广播、Redis 在线状态、MySQL 消息持久化、Outbox 可靠发布、DLQ 死信隔离与基础可观测性。

## 核心能力

- 用户注册、登录、JWT 认证、用户搜索、在线状态查询（搜索/在线状态均需登录）
- **JWT 主动失效**：登出 / 改密后旧 token 立即 401（Redis 黑名单 + 用户级失效时间戳）
- **一对一私聊**：WebSocket 精准推送、消息持久化、会话列表、未读标记、已读回执
- **群组聊天**：建群、邀请、入群申请、审批、踢人、转让群主、解散、退群
- **群消息可靠投递**：MySQL 事务写 `chat_group_messages` + `message_outboxes`，后台轮询发布 RabbitMQ fanout，每实例独立队列
- **群详情低延迟**：成员/消息 DTO 直接带 username，前端 3 个请求并行，无 N+1 补查
- **WebSocket 实时通信**：Hub 管理长连接、心跳、互踢、慢连接主动降级断开
- **断线消息补偿**：重连后按游标增量拉取（`/auth/message/sync`），按 ID 去重合并
- **Redis 在线状态**：Lua 原子操作 + TTL 兜底 + 分布式实例路由（`route:{uid}`）
- **群在线成员分片缓存**：`hash(uid) % 16` 分片，推送时 16 个 goroutine 并行
- **消费端幂等**：Redis SETNX 去重，坏消息 Nack 进 DLQ 隔离
- **演示数据**：`cmd/seed` 一键写入 demo 用户 + 群聊/私聊历史，登录页「体验 Demo」免注册进入
- `/metrics` Prometheus 文本指标 + pprof 性能分析 + `/healthz` `/readyz` 探活

## 技术栈

- Go 1.25 / Gin / GORM / golang-migrate
- MySQL 8 / Redis 8 / RabbitMQ（amqp091）
- Gorilla WebSocket / golang-jwt / Viper / zap

## 架构分层

```text
router -> middleware -> handler -> service -> dao -> domain model
```

- `router`：HTTP/WebSocket 路由注册
- `middleware`：JWT、限流（内存令牌桶）、CORS、request id、日志、panic recovery
- `handler`：参数绑定、调用 service
- `service`：业务编排、缓存、MQ、实时推送
- `dao`：MySQL/Redis 数据访问（接口 + GORM 实现）
- `domain/model`：GORM 实体
- `dto`：请求/响应/事件结构
- `pkg/core`：MySQL/Redis/RabbitMQ/日志/响应/迁移/metrics 基础设施

## 目录结构

```text
.
├── cmd/server              # 程序启动入口
├── cmd/bench               # 压测工具
├── cmd/dlqtool             # 死信队列重放工具
├── config                  # 配置加载与 config.yaml
├── internal/container      # 依赖注入装配
├── internal/dao            # MySQL/Redis 数据访问
├── internal/domain/model   # GORM 模型
├── internal/dto            # 请求/响应/事件 DTO
├── internal/handler        # HTTP/WebSocket handler
├── internal/middleware     # 中间件
├── internal/router         # 路由注册
├── internal/service        # 业务逻辑
├── pkg/core                # 基础设施（含 migrations/，embed 进二进制）
├── reliability.md          # 可靠性/压测/告警/演练手册
└── README.md
```

## 启动流程

`cmd/server/main.go` 启动时：

1. 加载配置（`config.yaml`，支持 `APP_*` 环境变量覆盖）
2. 初始化 MySQL、Redis
3. 执行 golang-migrate 版本化迁移（embed SQL）
4. 装配依赖注入容器
5. 初始化 RabbitMQ、WebSocket Hub、consumer、Outbox publisher（含断线重连、指数退避）
6. 注册 Gin 路由，启动 HTTP 服务（:19999）+ pprof/metrics（:6060）
7. 支持 SIGTERM 优雅关闭

## 群消息链路

```text
POST /auth/groups/:id/messages  (或 WS 发送 group_message)
  -> 校验群状态 normal、用户是 active 成员
  -> MySQL 事务：写 chat_group_messages + 写 message_outboxes（pending）
  -> 立即返回（响应只花写库时间，不等推送）
  -> OutboxService 每 2s 轮询，FOR UPDATE SKIP LOCKED 抢占到期记录
  -> 发布到 RabbitMQ fanout exchange（消息持久化 + 手动 ACK）
  -> 每实例独占队列各收一份广播
  -> consumer：SETNX 幂等去重 -> 查本机在线群成员（活跃∩在线，16 分片）
  -> 16 个 push worker 并发推 WebSocket
```

失败路径：发布失败指数退避重试（2s→…→64s 封顶，10 次后 failed）；坏消息 Nack(false,false) 进 `<queue>.<instance_id>.dlq` 死信队列，不阻塞 worker pool。

## WebSocket

```text
GET /ws  （token 经 Sec-WebSocket-Protocol 子协议传递，不进 URL 日志）
```

客户端发：

```json
{ "type": "group_message", "group_id": 1, "content": "你好" }
{ "type": "ping" }
```

心跳 30s 一次，在线状态 TTL 90s。每个连接独立 `readPump`/`writePump`，Send channel 容量 256，满了说明客户端读取过慢，服务端主动断开该连接，避免拖垮整群推送。

## Redis 用途

- `online:{uid}` / `route:{uid}` / `instance:{id}:users`（Lua 原子维护，90s TTL）
- 群 active 成员缓存 + 群在线成员 16 分片缓存
- 消费端幂等去重（`SETNX group_message_processed:{messageID}`）
- 用户资料缓存

## 路由概览

公开（无需登录）：`POST /user/register`、`POST /user/login`、`GET /user/register/config`、
`POST /user/demo-login`（需 `demo.enabled=true`）、`GET /ws`、`GET /healthz`、`GET /readyz`

登录后（`Authorization: Bearer <token>`）：

- 用户：`GET/PUT /auth/user/*`、`GET /auth/user/search`、`GET /auth/user/online/list`、
  `GET /auth/user/{id}/online`、登出、改密、角色管理
- 私聊：`GET /auth/message/conversations`、`GET /auth/message/list`、`GET /auth/message/sync`、`POST /auth/message/send`、`DELETE /auth/message/{id}`
- 群聊：`/auth/groups/*`（建群、成员管理、邀请/入群申请审批、退群、解散、发群消息、查群消息）
- 管理员：`/auth/admin/user/*`

> 用户搜索/在线状态必须登录，避免未认证枚举用户与在线状态（评审 P0 修复）。

## 数据库

基于 golang-migrate，SQL embed 进二进制：

- `pkg/core/migrations/*.sql`，启动时自动应用未执行版本
- DSN 需带 `multiStatements=true`
- 加表/改表追加新版本迁移文件，不改历史迁移

## 本地运行

需要 MySQL / Redis / RabbitMQ（可用仓库根目录 `docker-compose.yaml` 一键起依赖）。

```bash
go mod tidy
go run ./cmd/server
```

## 演示数据 / 体验账号（面试演示必做）

新注册账号默认是空应用。执行一次种子命令，即可得到可直接演示的完整数据：

```bash
# 本地：与 server 使用同一套 APP_* 环境变量
go run ./cmd/seed

# 服务器：复用 systemd 的环境变量文件
set -a; . /opt/go-admin/go-admin.env; set +a
/opt/go-admin/seed
```

写入内容（幂等，可重复执行）：

- 用户：`demo` / `alice` / `bob` / `carol`（默认密码 `Demo@123456`）
- 群「DI 产品体验群」：demo 群主、alice 管理员，含 12 条群聊历史
- demo 与 alice/bob/carol 的 10 条私聊（带未读红点）

前端登录页会出现「体验 Demo（免注册）」按钮：服务端 `demo.enabled=true` 时
`POST /user/demo-login` 免密登录 demo 账号；如果不需要演示入口，把 `demo.enabled` 设为 false。

清理评审遗留测试账号（会级联删除其群/消息，需显式确认）：

```bash
go run ./cmd/seed -purge-users-prefix review_ -yes
go run ./cmd/seed -purge-dissolved-groups -yes   # 清理已解散群
```

## 注册风控（公网部署建议开启）

- 注册接口限流：每 IP 20 秒 1 次（连续第二个请求直接 429），成功/失败都有审计日志
- 邀请码：配置 `auth.register_invite_code`（或 `APP_AUTH_REGISTER_INVITE_CODE`）后，
  注册必须携带 `invite_code`；前端会通过 `GET /user/register/config` 自动显示邀请码输入框
- 演示/生产环境建议两者同时开启，避免接口被批量注册脚本刷爆

## 相关文档

- [reliability.md](./reliability.md)：Outbox/DLQ 说明、metrics 指标、压测与故障演练
- [docs/deploy-native.md](./docs/deploy-native.md)：生产裸机部署（二进制 + systemd）
- [docs/deploy-2c2g.md](./docs/deploy-2c2g.md)：2C2G 小内存服务器调优
- [future.md](./future.md)：新功能分层开发模板
- [面试.md](./面试.md) / [interview_qa.md](./interview_qa.md)：面试复习手册
