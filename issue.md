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
- [x] [P0-1] Snowflake 死代码 → 已删除 `pkg/utils/snowflake.go`
- [x] [P1-1] 无优雅关闭 → `signal.NotifyContext` + `http.Server.Shutdown` + `Container.Shutdown`（`cmd/server/main.go`/`internal/container/container.go`）
- [x] [P1-2] RabbitMQ 无断线重连 → `RunReconnectLoop` 指数退避 + PublishChan/ConsumeChan 方法化（`pkg/core/rabbitmq.go`）
- [x] [P1-5] DAO 吞错 → `interact_dao.go`/`social_dao.go` 用 `errors.Is(err, gorm.ErrRecordNotFound)` 区分；outbox 标记失败补日志
- [x] [P2-1] 死代码 → 删除 `internal/gateway/`、`chat_service.go`、`consumer_pool.go`；go.mod 清理 streadway/amqp
- [x] [P2-3] N+1 查询 → 会话列表改批量 `IN` + `GROUP BY`（`message_dao.go`）
- [x] [P2-4] QUIC → 整体移除（`internal/gateway/` 删除），实时通信统一 WebSocket
- [x] [P0-2] CORS 反射 Origin → 白名单校验，命中才回显 Origin（`middleware/cors.go`，配置 `cors_origins`）
- [x] [P0-3] JWT 经 URL Query → WS 握手改 `Sec-WebSocket-Protocol` 子协议优先，兼容 Bearer / `?token=` 兜底（`handler/ws_handler.go`）
- [x] [P0-4] 明文密钥入库 → `config/config.yaml` 加入 `.gitignore` 并 `git rm --cached`，保留 `config.example.yaml`
- [x] [P1-3] 限流器内存泄漏 → 定时清理 goroutine，10 分钟清除超时未访问 bucket（`middleware/rate_limit.go`）
- [x] [P1-4] 健康检查 → 新增 `GET /healthz`（存活）与 `GET /readyz`（MySQL/Redis/RabbitMQ 连通，失败 503）（`router/router.go`）
- [x] [P2-2] Context 反模式 → service 删除 struct ctx 字段，改方法参数 `ctx context.Context` 按请求传递（user/post/interact/message service）
- [x] [P2-5] 两套在线状态 → 收敛为 PresenceService 唯一权威，删除 `user:online` Set 读写（`service/presence_service.go`）

---

## P0 — 安全与正确性

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

### [P1-3] 令牌桶限流器内存泄漏
- **位置**：`internal/middleware/rate_limit.go`
- **问题**：`defaultLimiter.buckets` map 只增不减，不同 IP/user 的 bucket 永久驻留，长期运行内存持续增长。
- **建议**：加定时清理 goroutine，删除 `lastRefill` 超过阈值（如 10 分钟）未访问的 bucket。

### [P1-4] 缺少健康检查端点 —— ✅ 已修复（见顶部 ✅ 区）

### [P1-5] DAO 层大量忽略 error —— ✅ 已修复（见顶部 ✅ 区）

---

## P2 — 架构与代码质量

### [P2-2] Context 反模式
- **位置**：多个 service（`ctx` 作为 struct 字段持有）
- **问题**：请求级 deadline/cancellation 无法贯穿 HTTP → Service → DAO → DB/Redis。
- **建议**：改为方法参数 `func (s *XService) Foo(ctx context.Context, ...)`，按请求传递。

### [P2-3] N+1 查询 —— ✅ 会话列表已修复（见顶部 ✅ 区）；`social_service.go` 待核对

### [P2-5] 两套在线状态并存（冗余） —— ✅ 已修复（见顶部 ✅ 区）

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

### [P3-1] 测试体系缺失 —— ⚠️ 部分完成（S3-12 单元测试 + S3-13 DAO 集成测试已落地，见 `pkg/core/*_test.go` / `internal/service/*_test.go` / `internal/dao/integration_test.go`；Service 层 Mock 与更多覆盖待补）

### [P3-2] 无 CI/CD
- **问题**：无 GitHub Actions，无自动 lint/build/test。
- **建议**：加 `.github/workflows/ci.yml`（`go vet` + `go build ./...` + `go test ./...`）。

### [P3-3] 无 API 文档
- **问题**：路由散落在 `router.go` 注释，无 Swagger/OpenAPI。
- **建议**：引入 `swaggo/swag` 生成文档（已有 Swagger UI 容器）。

### [P3-4] 数据库迁移无版本管理 —— ✅ 已修复（S3-15：golang-migrate + embed SQL，见 `pkg/core/migrations/`）

### [P3-5] 日志/配置/观测优化 —— ⚠️ 部分完成（S3-16：已换 zap 结构化日志 + 级别/格式配置；配置热更新、pprof 监听地址可配置化仍待做）

---

## 附：与其它文档的关系

- 修复进展对照 `docs/project-review.md` 第三部分（Sprint 计划）。
- 面试相关的「坑」见 `面试.md` 第 5 节，修复后记得同步更新该节措辞（P0-1 Snowflake、P2-4 QUIC 均已处理并同步）。
