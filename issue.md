# Issue 清单（待修复）

> 基于当前代码核对 + `docs/project-review.md` 审计整理，按优先级排序：
> **P0**（安全/正确性）> **P1**（可靠性/健壮性）> **P2**（架构/代码质量）> **P3**（工程化/测试）。
> 每项含「位置 / 问题 / 建议」，修复后勾选即可。

---

## ✅ 已修复（记录，避免重复排查）

- [x] WebSocket CheckOrigin 全放通 → 已加 Origin 白名单（`handler/ws_handler.go`）
- [x] RWMutex 慢客户端死锁 → 已改异步投递 Unregister 通道（`service/ws_hub.go`）
- [x] BroadcastMessage channel 二次 close panic → 已移除 default 分支 close（`service/ws_service.go`）
- [x] User.Password `json:"-"` 标签错位 → 已移到 gorm 标签外（`domain/model/user.go`）

---

## P0 — 安全与正确性

### [P0-1] Snowflake 是死代码（简历风险）
- **位置**：`pkg/utils/snowflake.go`
- **问题**：`NextID()` 全项目零调用，所有 ID 实际是数据库自增；且 `workerID=1`、`datacenterID=1` 硬编码，多实例会撞 ID；时钟回拨直接返回 0 无兜底。
- **影响**：简历若写「分布式唯一 ID」会被问穿；即使接入也不正确。
- **建议**：二选一 —— ① 直接删除该文件并同步简历；② 真接入：workerID/datacenterID 改为按 `INSTANCE_ID` 派生 + 时钟回拨处理，再让消息/帖子 ID 走 `NextID()`。

### [P0-2] CORS 反射 Origin 且允许 Credentials
- **位置**：`internal/middleware/cors.go`
- **问题**：`Access-Control-Allow-Origin` 直接反射请求 Origin，同时 `Allow-Credentials: true`，存在 CSRF/凭证滥用风险。
- **建议**：改为配置化的白名单校验，命中才回显 Origin；不命中返回 `*` 且不带凭证。

### [P0-3] JWT Token 经 URL Query 传递
- **位置**：`internal/handler/ws_handler.go`（`c.Query("token")`）
- **问题**：token 出现在 URL，会被访问日志、反向代理、浏览器历史记录泄漏。
- **建议**：WebSocket 握手改为 `Sec-WebSocket-Protocol` 子协议或 `Authorization` 头传递；至少文档里说明风险。

### [P0-4] 明文密码/密钥提交 Git
- **位置**：`config/config.yaml`
- **问题**：MySQL/Redis/RabbitMQ 密码、JWT secret 明文入库。
- **建议**：把 `config.yaml` 加入 `.gitignore`，保留 `config.example.yaml`（已支持 `APP_*` 环境变量覆盖），运行时注入。

---

## P1 — 可靠性与健壮性

### [P1-1] 无优雅关闭
- **位置**：`cmd/server/main.go`
- **问题**：无 SIGTERM/SIGINT 处理，进程被 kill 时 WebSocket 连接、MQ consumer、Redis 在线状态无清理，残留脏状态。
- **建议**：`signal.NotifyContext` + `http.Server.Shutdown`，依次：停收新连接 → 排空请求 → `WSHub.Shutdown`/`Presence.Shutdown` 清理在线状态 → 关 MQ/DB/Redis。

### [P1-2] RabbitMQ 无断线重连
- **位置**：`pkg/core/rabbitmq.go`
- **问题**：`amqp091.Dial` 后未监听 `NotifyClose`，MQ 抖动/重启后连接断开，服务需重启才能恢复。
- **建议**：监听 `conn.NotifyClose` + `channel.NotifyCancel`，用指数退避自动重连重建 Publish/Consume Channel 并重新声明交换机/队列。

### [P1-3] 令牌桶限流器内存泄漏
- **位置**：`internal/middleware/rate_limit.go`
- **问题**：`defaultLimiter.buckets` map 只增不减，不同 IP/user 的 bucket 永久驻留，长期运行内存持续增长。
- **建议**：加定时清理 goroutine，删除 `lastRefill` 超过阈值（如 10 分钟）未访问的 bucket。

### [P1-4] 缺少健康检查端点
- **位置**：`cmd/server/main.go` / `internal/router/router.go`
- **问题**：无 `/health`、`/ready`，Docker Compose 用 `service_started` 判断不准确，无法用于负载均衡探活。
- **建议**：加 `/health`（进程存活）和 `/ready`（检查 DB/Redis/MQ 连通性）。

### [P1-5] DAO 层大量忽略 error
- **位置**：`internal/dao/interact_dao.go`、`social_dao.go` 等
- **问题**：`_` 吞掉错误，失败静默，难以定位数据不一致问题。
- **建议**：统一返回 `error`，Service 层统一处理并打日志/记 metrics。

---

## P2 — 架构与代码质量

### [P2-1] 死代码/未接入模块（清理）
- **位置**：`internal/gateway/`（QUIC）、`internal/service/chat_service.go`、`internal/mq/consumer_pool.go`
- **问题**：QUIC 网关、ChatService、ConsumerPool 完整实现但 `container.go`/`main.go` 均未引用；ConsumerPool 还用废弃的 `streadway/amqp`。
- **建议**：逐个决定「接入 or 删除」。QUIC 若要接入：解决鉴权弱问题（见 P2-4）。同时移除 `go.mod` 里 `streadway/amqp` 冗余依赖。

### [P2-2] Context 反模式
- **位置**：多个 service（`ctx` 作为 struct 字段持有）
- **问题**：请求级 deadline/cancellation 无法贯穿 HTTP → Service → DAO → DB/Redis。
- **建议**：改为方法参数 `func (s *XService) Foo(ctx context.Context, ...)`，按请求传递。

### [P2-3] N+1 查询
- **位置**：`internal/service/social_service.go`、`message_service.go`
- **问题**：列表场景循环单条查用户，DB 压力随列表长度线性增长。
- **建议**：收集 ID 后用 `WHERE id IN (...)` 批量查询，再内存组装。

### [P2-4] QUIC 鉴权弱 + 慢客户端处理弱
- **位置**：`internal/gateway/quic_session.go`
- **问题**：① 第一条 stream 直接发 uid 字符串，无 JWT 校验；② `Enqueue` 队列满只返回 false 丢弃，不主动踢连接（WebSocket 侧会踢）。
- **建议**：鉴权复用 JWT；慢客户端对齐 WebSocket 策略——队列满时关闭会话。

### [P2-5] 两套在线状态并存（冗余）
- **位置**：`internal/service/ws_hub.go`（`SAdd user:online`）+ `internal/service/presence_service.go`（Lua）
- **问题**：`user:online` 全局集合（供 `is_online` 展示）与 Presence（route/实例/下线）职责重叠，维护两份易不一致。
- **建议**：收敛为 Presence 一套，`is_online` 由 `online:{uid}` 是否存在派生。

### [P2-6] Model 层泄漏 Gin binding 标签
- **位置**：`internal/domain/model/post.go` 等
- **问题**：数据库模型上挂 HTTP 参数绑定标签，DB 层耦合 HTTP 框架。
- **建议**：binding 标签全部移到 `dto`，Model 只留 gorm/json 序列化标签。

### [P2-7] 拼写错误 + 未注册路由
- **位置**：`internal/gateway/sesssion_manager.go`（三连 s）、`internal/dto/group_dto.go`（`GroupMesssageListQuery`）
- **问题**：文件名/结构体拼写错误；`CommentHandler.GetPublicUserComments` 已实现但路由未注册。
- **建议**：重命名修正拼写；给 `GetPublicUserComments` 补注册或删除。

---

## P3 — 工程化与测试

### [P3-1] 测试体系缺失
- **问题**：0 个 `*_test.go`，无单元/集成测试。
- **建议**：优先补 Service 层（Mock DAO）与核心链路（发送群消息 → outbox 落库）测试。

### [P3-2] 无 CI/CD
- **问题**：无 GitHub Actions，无自动 lint/build/test。
- **建议**：加 `.github/workflows/ci.yml`（`go vet` + `go build ./...` + `go test ./...`）。

### [P3-3] 无 API 文档
- **问题**：路由散落在 `router.go` 注释，无 Swagger/OpenAPI。
- **建议**：引入 `swaggo/swag` 生成文档（已有 Swagger UI 容器）。

### [P3-4] 数据库迁移无版本管理
- **问题**：裸用 `GORM AutoMigrate`，不可回滚、无历史。
- **建议**：引入 `golang-migrate` 或 `goose`。

### [P3-5] 日志/配置/观测优化
- **问题**：标准库 `log`（无级别/结构化/轮转）；无配置热更新；`pprof` 绑定 `127.0.0.1` 导致 Docker 容器内宿主机不可达。
- **建议**：换 `zerolog`/`zap`；配置热更新（可选）；pprof/metrics 监听地址可配置，Docker 用 `0.0.0.0:6060`。

---

## 附：与其它文档的关系

- 修复进展对照 `docs/project-review.md` 第三部分（Sprint 计划）。
- 面试相关的「坑」见 `面试.md` 第 5 节，修复后记得同步更新该节措辞（尤其 P0-1 Snowflake）。
