# DI 项目完整梳理：现状盘点 → 问题拆解 → 迭代路线

---

## 第一部分：现状盘点

### 1. 项目定位

**DI** 是一个以学习 Go 后端开发为目的的个人练习项目，功能上定位为**实时社交平台**（类似简化版 Twitter + 微信的混合体）。开发模式为"想到什么加什么"，采用增量叠加式开发，未经过系统性架构设计阶段。

**当前阶段判断**：从 Demo / 原型阶段向可演示作品过渡中。核心 CRUD 和实时通信链路已打通，但缺乏测试、CI/CD、生产级防护。

### 2. 技术栈全景

| 层级 | 选型 | 版本 | 热度/成熟度 |
|------|------|------|-------------|
| 语言 | Go | 1.25.8（Dockerfile 中使用，可能超前） | — |
| HTTP 框架 | Gin | v1.12.0 | ⭐⭐⭐⭐⭐ 主流 |
| ORM | GORM | v1.31.1 | ⭐⭐⭐⭐⭐ 主流 |
| 数据库 | MySQL | 8.x（Docker Compose） | ⭐⭐⭐⭐⭐ |
| 缓存 / 状态 | Redis | 8.x | ⭐⭐⭐⭐⭐ |
| 消息队列 | RabbitMQ | amqp091-go v1.11.0 | ⭐⭐⭐⭐ |
| WebSocket | gorilla/websocket | v1.5.3 | ⭐⭐⭐⭐⭐ |
| 认证 | golang-jwt | v4.5.2 | ⭐⭐⭐⭐⭐ |
| 配置 | Viper | v1.21.0 | ⭐⭐⭐⭐ |

**依赖说明**：`streadway/amqp`（已废弃）已移除，统一使用官方 `rabbitmq/amqp091-go`；`quic-go` 为 Gin v1.12.0 的传递依赖（indirect），非主动引入。

### 3. 已完成功能清单

#### ✅ 已完整实现

| 模块 | 功能点 | 完成度 |
|------|--------|--------|
| **用户系统** | 注册、登录（JWT）、登出、个人资料、角色管理、在线状态 | 90% |
| **帖子系统** | CRUD、列表/分页、热帖排行、话题搜索、按用户筛选 | 90% |
| **评论系统** | CRUD、按帖子/用户查询 | 85% |
| **互动系统** | 点赞/踩/收藏/分享（幂等切换）、计数查询 | 85% |
| **社交系统** | 关注/取关、拉黑/取消、关系查询、关注/粉丝列表 | 80% |
| **一对一私聊** | WebSocket 实时推送、消息持久化、对话列表、未读计数 | 85% |
| **群组系统** | 创建/解散、邀请/申请加入、审批、踢人、转让群主、设管理员 | 90% |
| **群聊消息** | WebSocket 发送、Outbox 可靠投递、RabbitMQ 异步推送、历史消息 | 85% |
| **中间件** | JWT 认证、令牌桶限流、CORS、Request ID、日志、Panic 恢复 | 80% |
| **在线状态** | Redis + Lua 原子操作、心跳续期、服务停机自动清理 | 90% |

#### ⚠️ 已实现但未接入（已清理）

> 以下模块已在 S0 阶段确认删除：QUIC 网关、ChatService、ConsumerPool（详见顶部 ✅ 修复记录）。

| 模块 | 说明 |
|------|------|
| ~~QUIC 网关~~ | ~~`gateway/quic_server.go` 等完整实现，但 `container.go` 和 `router.go` 未引用~~ ✅ 已删除 |
| ~~ChatService~~ | ~~`service/chat_service.go` 完整实现，未注入容器~~ ✅ 已删除 |
| ~~ConsumerPool~~ | ~~`mq/consumer_pool.go` 使用旧 AMQP 库实现，未被使用~~ ✅ 已删除 |
| **WSService 方法** | `BroadcastMessage`、`GetOnlineUsers`、`Shutdown` 已定义但从未调用 |
| **CommentHandler.GetPublicUserComments** | 方法已写，路由未注册 |

#### ❌ 明确缺失

| 功能 | 说明 |
|------|------|
| 单元测试 | 0 个 `*_test.go` 文件 |
| 集成测试 | 无 |
| 消息已读回执 | 群聊和私聊都没有 |
| 图片/文件上传 | 完全没有 |
| 消息推送（离线） | 断连后无推送通知 |
| API 文档 | 无 Swagger / OpenAPI |
| 优雅关闭 | 无 SIGTERM 处理 |
| 配置热更新 | 需重启 |
| 数据库迁移版本管理 | GORM AutoMigrate 裸用 |

### 4. 代码结构

```
f:\DI/
├── cmd/
│   ├── server/main.go          # ★ 主入口（200+ 行）
│   ├── bench/main.go           # 压测工具
│   └── dlqtool/main.go         # 死信队列处理工具
├── config/
│   ├── config.yaml             # ★ 主配置（含明文密码）
│   └── config.example.yaml     # 配置模板（支持 APP_* 环境变量覆盖）
├── internal/
│   ├── container/container.go  # ★ DI 容器（依赖装配中心）
│   ├── router/router.go        # ★ 路由注册（所有 API 端点）
│   ├── handler/                # 7 个 handler（HTTP 层）
│   ├── service/                # 15+ 个 service（业务逻辑）
│   ├── dao/                    # 10+ 个 dao（数据访问，含接口+实现）
│   ├── domain/model/           # 12 个 model（GORM 实体）
│   ├── dto/                    # 请求/响应 DTO
│   ├── middleware/              # 5 个中间件
├── pkg/
│   ├── core/                   # 基础能力（DB/Redis/MQ/Logger/Metrics/错误处理）
├── docs/                       # 文档目录
├── deploy/monitoring/          # Prometheus + Grafana 监控栈
├── js/                         # 前端静态文件（含 5.5MB 测试图片）
├── future.md                   # 新功能开发模板
├── intro.md                    # 项目介绍
├── interview_qa.md             # 面试问答
├── Dockerfile
├── docker-compose.yaml
└── .gitignore                  # 仅 3 条规则
```

**代码量**：约 80 个 Go 源文件，~12,000+ 行 Go 代码，0 行测试代码。

### 5. 当前可运行程度

**可以正常运行**：`cd cmd/server && go run main.go` 启动后，HTTP API、WebSocket、RabbitMQ 消费均能工作。已通过 `docker-compose.yaml` 提供一键启动依赖服务（MySQL + Redis + RabbitMQ + Swagger UI + 监控栈）。

**Dockerfile 存在问题**：`golang:1.25.8-alpine` 和 `alpine:3.21` 可能是超前版本号，需要确认是否存在。

### 6. 已知 Bug 与瓶颈（高优先级）

| 严重程度 | 问题 | 位置 |
|----------|------|------|
| ✅ 已修复 | ~~WebSocket CheckOrigin 全放通~~ — 已改为 Origin 白名单校验（localhost/127.0.0.1 + 同源） | `handler/ws_handler.go` |
| ✅ 已修复 | ~~RWMutex 潜在死锁~~ — 慢客户端改为异步投递 Unregister 通道，不再持读锁抢写锁 | `service/ws_hub.go` |
| ✅ 已修复 | ~~BroadcastMessage 可能 panic~~ — 不再在 default 分支 close channel | `service/ws_service.go` |
| ✅ 已修复 | ~~User.Password 结构体标签错误~~ — `json:"-"` 已移到 gorm 标签外 | `domain/model/user.go` |
| 🟠 HIGH | **错误全部吞掉** — `interact_dao.go` 和 `social_dao.go` 大量 `_` 忽略 error | dao 层多处 |
| 🟠 HIGH | **N+1 查询** — 社交/消息模块循环单条查询用户 | `service/social_service.go`, `message_service.go` |
| 🟠 HIGH | **令牌桶内存泄漏** — 限流器 bucket map 只增不减 | `middleware/rate_limit.go` |
| 🟠 HIGH | **Context 反模式** — `ctx` 作为 struct 字段而非按请求传递 | 多个 service |
| 🟠 HIGH | **无优雅关闭** — 进程被 kill 时连接/消费者/在线状态无清理 | `cmd/server/main.go` |
| 🟠 HIGH | **RabbitMQ 无重连机制** — 连接断开后需重启服务 | `pkg/core/rabbitmq.go` |
| 🟡 MEDIUM | **配置含明文密码已提交 Git** | `config/config.yaml` |
| 🟡 MEDIUM | **`.gitignore` 仅 3 条规则** — `.idea/`、`*.log`、`*.exe` 均未忽略 | 仓库根目录 |
| 🟡 MEDIUM | **JWT Token 经 URL Query String 传递** — 可被中间代理/日志记录 | `handler/ws_handler.go:29` |
| 🟡 MEDIUM | **CORS 反射 Origin + 允许 Credentials** — CSRF 风险 | `middleware/cors.go` |
| 🟡 MEDIUM | **错误消息泄漏内部细节** — `FailByError` 对非 BizError 直接输出 `err.Error()` | `pkg/core/response.go` |

---

## 第二部分：问题拆解

### 一、架构缺陷

#### 1.1 分层边界有泄漏

`future.md` 定义了清晰的 `handler → service → dao → model` 分层，但实际代码存在大量违规：

| 违规 | 位置 | 影响 |
|------|------|------|
| **Model 结构体上有 Gin binding 标签** | `model/post.go:18-28` | 数据库层耦合 HTTP 层，换框架或换协议就崩 |
| **DAO 接口暴露 GORM 类型** | `dao/interact_dao.go:16`（`WithTx(tx *gorm.DB)`） | Service 间接依赖 GORM |
| **Handler 直接写 SQL 逻辑** | `handler/post_handler.go:63-68`（手动解析 query 参数） | 绕过 DTO 层 |
| **Service 直接依赖 Gin Context** | 多处 Context 作为 struct 字段持有 | 与 HTTP 框架耦合 |

#### 1.2 群聊消息存在三条独立实现路径

| 路径 | 入口 | 验证逻辑 |
|------|------|----------|
| `GroupService.SendGroupMessage` | HTTP REST API | group status + member status |
| `WSHub.handleGroupMessage` | WebSocket 消息 | group status + member status |
| `ChatService.HandleGroupMessage` | MQ 消费（实现但未用） | group status + member status |

三份验证代码独立维护，规则变更时极易不一致。

#### 1.3 上下文传递反模式

```go
// 当前做法（反模式）
type PostService struct {
    ctx context.Context  // ← context 作为 struct 字段
}

// 正确做法
func (s *PostService) GetPost(ctx context.Context, id uint) (*Post, error) {
    // ctx 随请求传递，携带 deadline / cancellation
}
```

影响：HTTP 请求的 deadline 无法传递到下游 DB/Redis 调用，无法实现请求级超时控制。

#### 1.4 QUIC 网关"僵尸代码" —— ✅ 已移除

`internal/gateway/` 下 ~500 行完整实现的 QUIC 网关代码（QUICServer / QUICSession / RedisPresenceAdapter / SessionManager），`container.go` 和 `main.go` 中**完全没有引用**，属于实现了但从未接入的死代码。同样情况的 `mq/consumer_pool.go` 和 `service/chat_service.go`。

**处置（S0）**：三处全部删除，`go.mod` 清理 `streadway/amqp`；QUIC 鉴权弱、无客户端可演示、与主链路割裂，保留无收益。实时通信统一走 WebSocket + 断线消息补偿。

### 二、代码规范问题

#### 2.1 错误处理不一致

三种错误处理风格混杂：
```go
// 风格 1：吞掉错误
count, _ := repo.CountLikes(postID)

// 风格 2：打印日志但继续执行
fmt.Printf("mark messages as read failed: %v\n", err)

// 风格 3：返回 BizError
return core.NewBizError(http.StatusNotFound, "post not found")
```

同层同文件中三种风格共存，无法形成可靠的错误传播链。

#### 2.2 硬编码值散落各处

| 硬编码值 | 位置 |
|----------|------|
| JWT 过期时间 `1 * time.Hour` | `service/user_service.go:302` |
| `"admin"` / `"superadmin"` 角色字符串 | `middleware/admin.go:20` |
| 连接池 `MaxIdleConns(10)`, `MaxOpenConns(100)` | `pkg/core/mysql.go:35-37` |
| 热帖查询 limit 上限 `50` | `handler/post_handler.go:67` |
| WebSocket Send channel 容量 `256` | `service/ws_service.go:64` |
| 空头像 `"avatar": ""` | `service/social_service.go:192` 等 3 处 |

#### 2.3 中英文混用

响应消息中英文混杂：`"未登录"` / `"token无效"` / `"success"` / `"delete success"`。无 i18n 机制。

#### 2.4 其他代码质量问题

- `dto/group_dto.go:28` — `GroupMesssageListQuery` 拼写错误（三连 s）
- `model/user.go:16` — Password 字段的 `json:"-"` 写在 gorm 标签内，导致密码未从 JSON 输出中排除（安全漏洞 + 结构体标签错误）
- `gateway/sesssion_manager.go` — 文件名三连 s 拼写（应为 "session"）
- `handler/comment_handler.go` — `GetPublicUserComments` 方法已实现但路由未注册
- Post model 的 `PostStatusPublished` 常量与 GORM default 值大小写不一致
- `cmd/server/main.go:16` — 混用标准库 `log.Println` 和自定义 `appLogger`

### 三、性能问题

| 问题 | 严重程度 | 详情 |
|------|----------|------|
| **N+1 查询** | 🔴 高 | 社交关系列表、会话列表循环查单用户 |
| **LIKE '%topic%' 无法用索引** | 🟡 中 | 话题搜索必然全表扫描 |
| **令牌桶 map 无限增长** | 🟡 中 | 限流器从未清理过期 bucket，长期运行内存泄漏 |
| **热帖子查询** | 🟡 中 | 三连 LEFT JOIN 聚合子查询，数据量大时变慢 |
| **缓存击穿保护有 TOCTOU 竞态** | 🟡 中 | 释放锁 → 写缓存的窗口期可被并发请求穿透 |
| **群在线成员无分页** | 🟠 中低 | 千人大群的 SMEMBERS + 遍历推送 |
| **分片推送 workers 数竞态** | 🟠 中低 | `pushWorkers` 无并发保护 |

### 四、缺失模块

#### 4.1 测试体系（最大缺失）

**0% 测试覆盖率。** 没有单元测试、集成测试、端到端测试。80 个 Go 文件，0 个 `*_test.go`。

#### 4.2 CI/CD

无 GitHub Actions / Jenkins pipeline。无自动化构建、测试、部署。

#### 4.3 API 文档

无 Swagger / OpenAPI 规范。路由散落在 `router.go` 的注释中。

#### 4.4 数据库迁移管理

✅ 已改 golang-migrate（SQL 版本化）：迁移文件 `pkg/core/migrations/*.sql`（embed 进二进制），启动执行未应用迁移，`schema_migrations` 表记录版本，支持 down 回滚。历史问题"裸 AutoMigrate 不可回滚"已解决。

#### 4.5 配置管理

配置文件含明文密码提交 Git。无环境变量覆盖机制（虽有 `os.Getenv("INSTANCE_ID")` 但密码等不走环境变量）。

#### 4.6 日志系统

✅ 已改 zap（SugaredLogger）结构化日志：JSON/console 双格式（配置 `logger.format` 切换）、级别控制（配置 `logger.level`：debug/info/warn/error）、ISO8601 时间、请求中间件与 panic recovery 输出结构化字段。剩余可改进：日志轮转/归档（Lumberjack 等）与跨服务 trace_id 注入。

#### 4.7 健康检查

✅ 已实现：`GET /healthz`（进程存活）与 `GET /readyz`（MySQL SELECT 1 + Redis Ping + RabbitMQ 发布通道未关闭，任一失败返回 503），可用于 Docker healthcheck / 负载均衡探活。Docker Compose 可改用 `condition: service_healthy`。

#### 4.8 API 版本化

无版本前缀（如 `/v1/`），未来 API 变更需硬 break。

### 五、部署 / 调试痛点

| 痛点 | 说明 |
|------|------|
| **无优雅关闭** | SIGTERM 直接杀进程，连接/消费者/在线状态残留 |
| **RabbitMQ 断连无重连** | MQ 抖动后服务不可用，需重启 |
| **pprof 绑定 127.0.0.1:6060** | Docker 容器内无法从宿主机访问 |
| **Prometheus 配置 hardcode** | `host.docker.internal:6060` 在 Linux Docker 上不工作 |
| **日志全部 stdout** | 无文件输出、无日志收集、容器重启日志丢失 |
| **Dockerfile 版本号可能无效** | `golang:1.25.8-alpine` 需确认是否存在 |
| **无 Makefile / Taskfile** | 开发命令靠手工记 |

---

## 第三部分：长期发展规划

### 短期迭代（1-3 个小版本，预计 2-4 周）

#### Sprint 1：止血 — 修复已知高危 Bug（优先级最高）

```
☐ 修复 User.Password 结构体标签（json:"-" 移到外面）
☐ 修复 WebSocket CheckOrigin 安全策略
☐ 修复 RWMutex 潜在死锁（SendMessageToUser → dropSlowClient 路径）
☐ 修复 BroadcastMessage channel 二次 close panic
☐ 修复 password hardcode 暴露 → config.yaml 移出 Git，使用 .env 或环境变量
☐ 扩展 .gitignore（.idea/, *.log, *.exe, config/config.yaml, mysql-data/ 等）
```

#### Sprint 2：填坑 — 清理死代码 & 接入未使用模块（已完成 ✅）

```
☑ 删除 internal/mq/consumer_pool.go（含旧 AMQP 依赖清理）
☑ 删除 internal/service/chat_service.go
☑ 清理 streadway/amqp 废弃依赖
☑ 删除 QUIC 网关 internal/gateway/（鉴权弱 + 无客户端，做减法）
☐ 给 CommentHandler.GetPublicUserComments 注册路由
☐ 修复拼写错误（sesssion → session，Messsage → Message）
```

#### Sprint 3：规范化 — 代码质量基础建设

```
☐ 统一错误处理：所有 dao/service 不得用 _ 吞错误
☐ 消除 N+1 查询：社交/消息模块改为批量查询
☐ 令牌桶限流器加入定时清理过期 bucket 机制
☐ Context 传递规范化：ctx 从 struct 字段改为函数参数
☐ 所有硬编码值抽到配置（JWT TTL, 连接池参数, 限流参数等）
☐ 引入 golangci-lint，修复所有 warning
```

### 中期完善（预计 1-3 个月）

#### 架构重构

```
☐ 统一群聊消息入口：合并 3 条路径为 1 个 GroupChatService
☐ Model 层去 HTTP 依赖：移除所有 Gin binding 标签
☐ DAO 接口去 GORM 化：WithTx 改用 context-based transaction
☐ 引入 Wire 或手动重构 Container 为 Builder 模式
☐ 引入请求级 context 传递链（贯穿 HTTP → Service → DAO → DB/Redis）
```

#### 新增核心功能

```
☐ 消息已读回执（私聊 + 群聊 last_read_message_id）
☐ 离线消息推送通知（FCM / APNs 或至少 WebSocket 重连后补推）
☐ 图片/文件上传（MinIO / 阿里云 OSS）
☐ 消息撤回（2 分钟内）
☐ @提醒功能
☐ 消息搜索（Elasticsearch 或 MySQL 全文索引）
```

#### 可靠性提升

```
☐ RabbitMQ 连接自动重连（带指数退避）
☐ MySQL/Redis 连接池健康检查与自动恢复
☐ 优雅关闭（SIGTERM → 停收新请求 → 排空队列 → 清理在线状态 → 关闭连接）
☐ Outbox 表定时清理（已 published 超 7 天删除）
☑ 引入 golang-migrate 替代裸 AutoMigrate（S3-15 已完成）
```

#### 测试体系建设

```
☐ 为 DAO 层编写单元测试（SQLite :memory: 或 testcontainers）
☐ 为 Service 层编写 Mock DAO 的测试
☐ 为 Handler 层编写 HTTP 测试（httptest）
☐ 关键路径端到端测试（注册 → 发帖 → 评论 → 点赞完整流程）
☐ 目标：核心模块测试覆盖率 > 70%
```

#### DevOps

```
☐ 编写 Makefile（make dev / make test / make build / make docker）
☐ GitHub Actions CI（lint + test + build）
☐ 增加 /health /ready 端点
☐ 结构化日志（zerolog 或 zap）
☐ Swagger 文档自动生成（swaggo/swag）
```

### 长期拓展（预计 3-6 个月+）

#### 方向一：商业化

```
☐ 多租户架构（SaaS 化）
☐ 支付集成（会员订阅 / 打赏）
☐ 内容审核（敏感词过滤 + 图片审核 + AI 审核）
☐ 数据看板（DAU、留存、消息量、热门话题）
☐ 广告投放系统
☐ 合规需求（数据导出、账号注销、隐私协议）
```

#### 方向二：毕业设计方向

如果将此项目用于毕业设计，建议聚焦一个**技术深度点**：

```
选题建议：「基于消息可靠性设计的实时 IM 系统：Outbox + 断线补偿」

论文可展开的点：
  - Transaction Outbox 模式在分布式系统中的可靠投递保证
  - 断线消息补偿：游标增量同步 + 幂等去重（WS 先推后补）
  - 分片缓存 + Lua 原子操作的在线状态一致性保证
  - RabbitMQ 断线重连与指数退避

补充工作：
  - 弱网实验（tc netem 模拟丢包/抖动），对比补偿前后消息丢失率
  - 收集性能数据绘图表
  - 撰写论文（背景→相关工作→系统设计→实验评估→结论）
```

#### 方向三：求职作品集

如果作为求职作品集，需要提升**展示力**：

```
☐ 编写高质量的 README（架构图、技术决策记录 ADR、运行截图/GIF）
☐ 录制功能演示视频（3 分钟展示核心功能）
☐ 编写架构设计文档（为什么这样选型、做了哪些权衡）
☐ GitHub Profile 优化（Pin 项目、完善描述、添加 Topics 标签）
☐ 补充 3-5 篇技术博客（如"Go 中 Outbox 模式的实现"）
☐ 性能测试报告（wrk/k6 压测数据，展示 QPS/延迟/资源占用）
☐ 参与开源社区（给使用的库提 PR / issue）
```

#### 方向四：架构演进（技术深度）

```
☐ 微服务拆分（用户服务 / 内容服务 / 消息服务 / 网关服务）
☐ 引入 gRPC 做服务间通信
☐ Kafka 替代 RabbitMQ（高吞吐场景）
☐ Kubernetes 部署（编写 Helm Chart）
☐ Service Mesh（Istio）流量管理
☐ 分布式链路追踪（OpenTelemetry + Jaeger）
☐ 读写分离（MySQL 主从 + 分库分表）
```

---

## 附录：问题清单速查表

### 按严重程度分类

| 严重程度 | 数量 | 典型问题 |
|----------|------|----------|
| 🔴 CRITICAL | 0 | 原 4 项（WS Origin 全放通、RWMutex 死锁、channel panic、密码泄漏）已在 2026-09 修复 |
| 🟠 HIGH | 7 | N+1 查询、错误吞掉、内存泄漏、Context 反模式、无优雅关闭、无 MQ 重连、Outbox 无引用 |
| 🟡 MEDIUM | 8 | 明文密码入 Git、JWT URL 传递、CORS 反射、标签拼写错误、缺少 .gitignore |
| 🟢 LOW | 5 | 中文消息混用、log 混用、Dockerfile 版本号、魔法数字、未用方法 |

### 按模块分类

| 模块 | 问题数 | 主要问题 |
|------|--------|----------|
| `handler/` | 5 | WS Origin 全放通、JWT URL 传递、参数解析旁路 DTO、无分页 |
| `service/` | 8 | 死锁、N+1、错误吞掉、Context 反模式、TOCTOU 竞态 |
| `dao/` | 4 | 错误吞掉、LIKE 无法用索引、接口泄漏 GORM 类型 |
| `middleware/` | 4 | 令牌桶泄漏、CORS 反射、硬编码角色 |
| `model/` | 2 | Gin 标签泄漏、Password JSON 标签错误 |
| ~~`gateway/`~~ | 1 | ~~QUIC 代码完全未接入~~ ✅ 已删除 |
| `pkg/core/` | 3 | MQ 无重连、配置无默认值、错误消息泄漏 |
| `config/` | 2 | 明文密码入 Git、缺失配置校验 |
| `Dockerfile/compose` | 4 | Go 版本号不确定、弱密码、端口暴露、健康检查 |
| `.gitignore` | 1 | 仅 3 条规则 |

---

*本文档生成于 2026-07-01，基于对项目全部 Go 源文件的审计。建议每完成一个 Sprint 后更新一次。*

---

## 更新记录（2026-09-14 同步）

本次对照当前代码修正以下过时结论：

- ✅ 4 项 CRITICAL 已在当前代码中修复：WebSocket CheckOrigin 白名单、RWMutex 慢客户端死锁、BroadcastMessage channel 二次 close panic、User.Password JSON 标签。
- ✅ S0 收尾：删除 `pkg/utils/snowflake.go`、`internal/gateway/`（QUIC）、`internal/service/chat_service.go`、`internal/mq/consumer_pool.go`；清理 `streadway/amqp`。
- ✅ S0 工程化：优雅关闭（SIGTERM → Shutdown）、RabbitMQ 断线重连（指数退避）、统一错误处理（不泄漏内部错误、DAO 吞错修复）、会话列表 N+1 修复（批量 IN + GROUP BY）、断线消息补偿 sync 接口。
- `config.docker.yaml` 已改为 `config.example.yaml`（支持 `APP_*` 环境变量覆盖）。
- 其余 HIGH/MEDIUM 问题（CORS 反射 Origin、JWT 经 URL Query、令牌桶内存泄漏、健康检查缺失、Context 反模式、无测试、无迁移管理等）仍待 S1-S3 处理。
