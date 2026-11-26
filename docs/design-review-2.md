# DI 后端架构评审报告（第二轮 · 只读）

> 评审范围：`internal/` + `pkg/` 全部 74 个 `.go` 文件、`docs/openapi.yaml`、`docs/review-fixes.md`、`README.md`、迁移 SQL。
> 性质：设计评审（非 bug 排查），结论按严重程度排序，每条给 `文件:行号` 证据；无证据处标注「推测」。
> 结论口径：**真问题** vs **过度设计**，不堆砌建议。

---

## TL;DR（一页结论）

整体架构是**合格甚至偏上**的：`router → middleware → handler → service → dao → domain` 分层基本成立，handler 薄、service 有真实业务逻辑、DAO 是接口 + GORM 实现；群消息的**事务性 Outbox + 消费端 SETNX 幂等 + DLQ 隔离**这条主链路是本次评审最扎实的部分。JWT 主动失效（黑名单 + 用户级毫秒 `invalid_before`）也设计得对。

但有 5 处值得动手的问题，按价值排序：

| 级别 | 问题 | 一句话 | 关键证据 |
|---|---|---|---|
| **P1** | `marshalWithPool` 返回切片别名 pool 内存 | 并发下 WS 推送内容可能被别的 goroutine 覆盖（串包/竞态） | `ws_service.go:24-33` |
| **P1** | 私聊/群聊的 WS 与 HTTP 发送是**两套重复实现且行为漂移** | WS 路径缺防抖/自检/2000 字长校验 | `ws_hub.go:248-346` vs `message_service.go:220-265` |
| **P1** | **群消息没有断线补偿**（无 `after_id` 增量同步） | 离线/断线的群成员会永久错过实时推送，只能重新进群拉全量历史 | 私聊有 `/auth/message/sync`，群聊无 |
| **P2** | RabbitMQ 发布**无 publisher confirm** | outbox「published」= 套接字写成功，非 broker 持久化确认 | `group_message_publisher.go:39-53` |
| **P2** | `request_id` 未传播到异步链路 | consumer/outbox/WS 日志无 user_id/message_id，线上难定位到具体消息 | `group_message_consumer.go:116` |

**过度设计**（不必改，但要知道）：16 分片群在线缓存 + 每条消息 16 goroutine、cache-aside + 自旋锁的重缓存机制、`RequestID` 半成品去重字段——在当前单实例/演示规模下都是「为面试而设计」的复杂度，可保留但不值得再投入。

**文档滞后**（顺手修）：`openapi.yaml` 与 `000001_init` 仍含已删除的社区模块（Post/Comment/Interaction/Social），`000001` 建表 `000002` 又删表。

只做 3 处改造的推荐见文末 §6：**① 统一 WS/HTTP 发送链路 → ② 群消息增量同步 → ③ publisher confirm + 异步日志上下文**。

---

## §1 分层是否成立

**结论：成立。** handler 只做「绑定参数 → 调 service → 回响应」，service 承载业务编排，DAO 是接口 + GORM 实现。没有出现 service 反向依赖 Gin、handler 直接查库这类硬伤。

### 1.1 业务逻辑泄漏点（真实，但均为轻度）

| 位置 | 泄漏内容 | 严重度 |
|---|---|---|
| `internal/dao/group_gorm_dao.go:309-359` | `AcceptInvitation` / `ApproveJoinRequest` 内含「成员已存在则改 role/status，否则新建」的业务规则（upsert-if-exists） | 轻（放事务里做「写两个聚合」属可接受的仓储模式） |
| `internal/dao/outbox_dao.go:106-122` | `MarkFailed` 内含「retryCount>=10 → failed」的重试策略 | 轻（策略本可放 service） |
| `internal/handler/user_handler.go:38-44` | 注册成功/失败审计日志写在 handler | 轻（横切关注点，可接受） |

### 1.2 真正的问题：WS 与 HTTP 是两套发送实现（DRY 破坏 + 行为漂移）

私聊与群聊各有一条「校验 → 落库 → 推送」的完整链路，分别写在：

- HTTP 私聊：`message_service.go:220-265`（`SendMessage`）
- WS 私聊：`ws_hub.go:248-290`（`handleMessage`）
- HTTP 群聊：`group_message_service.go:13-65`（`SendGroupMessage`）
- WS 群聊：`ws_hub.go:292-346`（`handleGroupMessage`）

四条链路中，WS 版是「缩水版」，**缺失了 HTTP 版已有的保护**：

- HTTP 有重复提交防抖 `checkDuplicateSubmit`（`message_service.go:230-236`），WS 无；
- HTTP 有「不能发给自己」校验（`message_service.go:226-228`），WS 无；
- HTTP 群消息有 2000 字长上限（`group_message_service.go:18-20`），WS 无。

后果：同一个「发消息」动作，走 HTTP 和走 WS 的约束不一致，维护时改一处漏一处。这是本轮最值得收敛的结构性问题。

### 1.3 是否出现「纯转发 service」

几乎**没有**。多数 service 方法有真实校验与编排（`GroupService` 的 `getNormalGroup`/`getActiveMember`/`requireOwner` 复用抽取得不错）。仅有 `user_service.go:516-541`（`GetUserRole`/`BatchGetUserRoles`）近乎直通 DAO，属正常边界，不算问题。

### 1.4 死代码（顺带指出）

- `ws_service.go:168-194` `BroadcastMessage` —— 无任何调用点（grep 确认）。
- `group_gorm_dao.go:139-155` `UpsertMember` —— 接口与实现都在，service 层零调用。
- `event_dto.go:7` `RequestID` 字段 + `group_message_consumer.go:229-238` 的 `request_id` 去重回退 —— 事件构造从不赋值 `RequestID`（`outbox_helpers.go:13-25`），该分支恒不生效。

---

## §2 事务边界与并发一致性

### 2.1 做得对的：群消息「落库 + Outbox」同事务 ✅

`group_gorm_dao.go:384-401` `CreateGroupMessageWithOutboxBuilder` 用 `db.Transaction` 把 `chat_group_messages` 与 `message_outboxes` 一次写入——这是标准的 **Transactional Outbox** 模式，是「消息不丢」的根基，实现正确。

### 2.2 群成员变更：check-then-act 竞态（真问题，低危）

所有成员写操作都是「先读校验、后无守卫 UPDATE」：

- 踢人 `group_admin_service.go:101-144`：`getActiveMember`（读）→ 改内存 → `UpdateMember`（`Save` 全量覆盖）。
- 退群 `group_admin_service.go:207-232`：同样读改写。
- 设/撤管理员 `group_admin_service.go:13-62`：`validateAdminOperation`（多次 `FindMember`）→ `UpdateMember`。

问题：UPDATE 没有 `WHERE status='active'` 之类的乐观条件，也没有版本号。两个管理员并发踢同一人 / 踢人 vs 审批通过并发时，会出现「读到的状态已过期仍覆盖」。**影响**：这是低频管理操作，最坏是「被踢的人又加回来」或「已退群成员被二次标记」，不会破坏资金/权限安全，故判低危。真正的修复是给关键状态流转加 `WHERE` 守卫或乐观锁——但**建议标注为「多实例上线前才值得做」，当前单实例可暂缓**。

### 2.3 邀请/入群审批：双重提交竞态（真问题，低危）

`ReviewInvitation`（`group_invitation_service.go:69-127`）与 `ReviewJoinRequest`（`group_join_request_service.go:73-138`）在 service 层读 `Status == pending`（`:79-81` / `:79-81`），随后 DAO 用 `tx.Save(invitation)`（`group_gorm_dao.go:311`）全量写回，**没有 `WHERE status='pending'` 守卫**。两个审批请求并发都能通过 pending 检查、都写 accepted。因成员 upsert 幂等（不会重复建成员），净效果基本无害，属「重复审批」而非「数据损坏」，低危。

### 2.4 未读计数：无竞态（派生状态）✅

未读不落库，靠 `COUNT` 实时算（`message_dao.go:119-135`、`CountUnreadByPeers` `:153-172`），`MarkMessagesAsRead` 是原子 `UPDATE`（`:104-116`）。不存在「计数与消息不一致」的半成功状态。这是**做得对的选择**（省掉了 denormalize 计数表的复杂度与一致性问题）。

唯一小瑕疵：`GetMessageList` 里「先 `FindUnreadMessageIDs` 再 `MarkMessagesAsRead` 再发回执」（`message_service.go:142-153`）之间有一个小窗口——新到的消息可能被 `MarkMessagesAsRead` 一并标记已读，但没给它发已读回执。仅影响回执完整性，不影响数据。

### 2.5 登出吊销：fail-open 与 fail-closed 不对称（真问题，中危）

- 校验侧 `IsRevoked` 在 Redis 抖动时 **fail-open**（放行）：`middleware/jwt.go:86-87`、`ws_service.go:98-99`。
- 注销侧 `Revoke` 失败则 **fail-closed**（logout 直接 500）：`user_service.go:438-441`。

后果：Redis 不可用时，「登录态校验」继续放行（可用性优先），但「登出」返回失败，用户退出不了。这是自洽的取舍（登出若假装成功反而更糟），但值得在文档里显式声明，否则运维会困惑。

另一个多实例限制：`onUserKicked` = `wsService.KickUser` 只关闭**本实例**的 `hub.Clients`（`ws_service.go:158-166`）。改密后其他实例上同一用户的存量 WS 连接不会被踢，只能等到其下一次 HTTP 请求命中 `invalid_before` 才失效。当前单实例部署无影响，**多实例上线前必须补**（可用 Redis pub/sub 广播踢人）。

---

## §3 RabbitMQ 可靠性

### 3.1 做得对的 ✅

- 事务性 Outbox（§2.1），DB 是事实源，MQ 只是实时投递加速器。
- 消费端 SETNX 幂等：`group_message_consumer.go:191-211`，重复投递不会重复推送。
- 坏消息 `Nack(false,false)` 进 DLQ，不阻塞 worker pool：`:118`。
- DLQ 有重放工具：`cmd/dlqtool/main.go`。
- 队列/交换机/DLQ 拓扑声明正确，重连后重建：`pkg/core/rabbitmq.go:74-117`。

### 3.2 publisher confirm 缺失（真问题，中危）

`group_message_publisher.go:39-53` 用 `PublishWithContext` + `DeliveryMode:Persistent`，但**通道未开启 confirm mode**。`PublishWithContext` 返回 nil 只代表「消息写进了 TCP 套接字」，不代表 broker 已持久化。若在「写套接字成功」与「broker 落盘」之间断连，`outbox_service.go:73` 已把该条标 `published`，而消息实际丢了 → **实时推送静默丢失**。

缓解：消息本体仍在 MySQL（`chat_group_messages`），用户重进群拉历史能补回；所以「数据不丢」仍成立，「实时推送不丢」不成立。要堵这个洞，需给发布通道开 `Confirm` 并在 `Publish` 后 `WaitForConfirms`。

### 3.3 消费端顺序无保证（真问题，中危）

单队列按序投递，但 consumer 用一个 dispatcher 把 `deliveries` 灌进 `tasks` 缓冲、8 个 worker 并发拉取处理（`group_message_consumer.go:69-101`），**同一群的消息可能乱序推送**（消息 2 先于消息 1 到达客户端）。私聊有 `id` 排序兜底，但群聊实时推送无排序。客户端有 ID 去重，但去重≠排序。若聊天产品对「先后可见」敏感，需按 `group_id` 做分区顺序消费或客户端按 `message_id` 排序合并。

### 3.4 幂等去重的边界（轻，标注即可）

- `markProcessing` 的「processing」TTL 是 5 分钟，「done」是 7 天（`group_message_consumer.go:25-26`）。若某条消息处理超 5 分钟（极端），或 DLQ 在 5 分钟内被 `dlqtool replay` 重放，同一 `message_id` 会被判定为「processing」而 Ack 掉、**不推送** → 丢一次实时推送。概率极低，属「重放工具与幂等窗口的配合」注意点。
- `handleDelivery` 多数错误路径会 `clearProcessing`，但 `markProcessing` 自身出错（Redis SETNX 失败）时直接返回 err → Nack 进 DLQ，此时若 key 已写入则残留 5 分钟，与上一条同源。

### 3.5 WebSocket 推送失败会不会丢？

会丢「实时推送」，但**私聊可补偿、群聊有缺口**：

- 慢客户端：`ws_hub.go:75-89` 通道满即丢弃并异步断开——这是刻意的背压设计，正确。
- 私聊：离线/断线由 `/auth/message/sync`（`message_dao.go:176-186`）增量补偿 ✅。
- **群聊：无 `after_id` 增量接口**，只有 `GET /auth/groups/:id/messages` 全量分页。群成员离线或推送时被断开，期间错过的群消息只能靠「重新进群拉全量历史」恢复，没有游标式补偿。这是**与私聊能力不对等**的缺口，也是 §6 的第二处改造。

---

## §4 WebSocket 状态管理

### 4.1 连接/鉴权/互踢：清晰 ✅

- 鉴权入口三选一（子协议/头/query）：`ws_handler.go:80-92`，HMAC 防算法混淆 + 主动失效检查：`ws_service.go:65-106`。
- 互踢：`registerClient` 关旧连、立新连（`ws_hub.go:124-154`），`unregisterClient` 用 `current == client` 防误删新连接（`:159`），处理得当。
- 优雅关闭：`Shutdown` 关 `Send` + `Conn`、清 Redis（`ws_hub.go:103-120`、`presence_service.go:192-206`）。

### 4.2 心跳/断线：基本成立，有冗余

- 客户端 JSON `{"type":"ping"}` 续期：`ws_hub.go:348-357` → `presence.RefreshOnline`，presence TTL 90s，read deadline 60s。
- **冗余/误导**：`ws_service.go:111-114` 设置了 `SetPongHandler`，但服务端**从不发 gorilla 协议层 ping**，所以 pong handler 实际是死代码；真正的心跳是应用层 JSON ping。不影响正确性，但易误导后人。

### 4.3 内存泄漏风险：可控，无风暴

- `Clients` 以 `UserID` 为 key，单用户单连接（互踢），不会无限增长。
- 慢客户端被异步踢（`ws_hub.go:83-88`），`Unregister` 通道满时起 goroutine 兜底（`:87`）——理论上有 goroutine 堆积风险，但 Hub 事件循环只做 map 操作、排空极快，风险极低。
- 无「广播风暴」：群推送按成员分片、按在线过滤（`group_message_consumer.go:244-297`），不会全量广播。

### 4.4 真正的并发 bug：`marshalWithPool` 串包（P1，本轮最该修的一处）

`ws_service.go:24-33`：

```go
buf := bufPool.Get().(*bytes.Buffer)
buf.Reset()
defer bufPool.Put(buf)          // ← 函数返回时立即把 buf 还回池子
...
return buf.Bytes(), nil         // ← 返回的 []byte 别名 buf 底层数组
```

`buf.Bytes()` 返回的切片别名 `buf` 的底层数组；`defer bufPool.Put(buf)` 在 return 后立刻把同一个 `*bytes.Buffer` 还回 `sync.Pool`。下一个 goroutine `Get()` 拿到同一 buffer，`Reset()`（`b.buf = b.buf[:0]`，保留底层数组）再 `Encode`，会**覆写同一块内存**。调用方（`ws_hub.go:268/280/352/360`、`ws_service.go:130`）把返回的 `[]byte` 塞进 `client.Send` 通道，在 `writePump` 真正 `WriteMessage` 之前，内容可能已被并发的另一次 marshal 覆盖。

后果：高并发推送时消息内容**串包/错乱**，且是间歇性的（池中 buffer 数 < 并发数时才触发）——`go test -race` 可复现。修复一行即可：返回前拷贝（`append([]byte(nil), buf.Bytes()...)`），或干脆不池化（编码成本远低于这层风险）。**这条是上一轮修复遗漏的并发正确性问题，建议立即修。**

---

## §5 错误处理与可观测性

### 5.1 错误码：统一但粗粒度

`BizError{Code, Msg}` + `FailByError`（`response.go:44-78`）统一了「业务错误走 HTTP 状态码 + 内部错误进日志、响应只回 500」的范式，是对的。但 `Code` 复用的是 **HTTP 状态码**而非业务错误码枚举，定位「群已解散(410)」「参数错误(400)」只能靠文案，无稳定机器可读错误码。对当前规模够用，接入告警/前端分支时再补枚举即可。

### 5.2 日志：结构化但缺「请求/消息/用户」上下文（真问题，中危）

- `request_id` 中间件存在（`request_id.go`），但只在 `request_logger.go:27` 与 `custom_recovery.go:17` 两条日志里用到。
- **`core.Z()` 是全局 logger，业务与异步链路全部不带 request_id/user_id**：
  - consumer 出错 `group_message_consumer.go:116` 只打 `consume group message event failed`，**无 message_id / group_id / user_id**；
  - outbox 出错 `outbox_service.go:58/67` 只打 `id=%d`；
  - WS 出错 `ws_hub.go:262` 只打 error。

后果：HTTP 请求能定位到「哪个请求、哪个用户」（访问日志有 request_id + user_id），但**一旦进入 MQ/outbox/WS 异步路径，就断了**——线上「某条群消息没推出去」时，无法从日志串起「哪条消息、哪个群、哪个实例」。这是「可观测性」最实际的短板。

### 5.3 具体小瑕疵

- `message_service.go:148` 用 `fmt.Printf("mark messages as read failed")` 绕过 zap，走 stdout、无上下文、无级别。
- `/metrics`（`metrics.go`）只有计数器，无请求级 latency/histogram，无法从指标告警 P99（latency 只在日志里）。

### 5.4 trace

无分布式追踪（无 OpenTelemetry/Jaeger）。`request_id` 存在但未跨「HTTP → DB/MQ → consumer」传播。**判定：** 对当前单实例单体 + 单条异步链路，暂不需要上全量 tracing；但 §5.2 的「异步日志带上下文」是 tracing 的廉价替代，应优先做。

---

## §6 只做 3 处改造，收益最大

> 均给出理由 / 影响范围 / 工作量 / 是否值得。以下按收益排序。

### ① 统一私聊与群聊的「WS / HTTP 发送」链路

- **理由**：当前四套重复实现（§1.2）导致行为漂移——WS 缺防抖、缺自检、缺 2000 字长校验。统一后一处真相，三个缺口自动修复，未来改消息规则只改一处。
- **影响范围**：`ws_hub.go:248-346`（把 `handleMessage`/`handleGroupMessage` 改为委托 `MessageService.SendMessage` / `GroupService.SendGroupMessage`）、`container.go`（把 service 注入 WSClient/Hub）、`ws_service.go`（推送收口）。
- **工作量**：中（约 1–2 天），需小心处理「WS 发消息要在落库后推给双方 + 回执」的时序，但逻辑清晰。
- **是否值得**：**值得**。这是所有改造里「消除存量复杂度」性价比最高的一件，且顺带把 §4.4 的 `marshalWithPool` 串包问题一并修掉（统一后 push 走一处，收口时顺手改成拷贝语义）。

### ② 群消息增量同步（`after_id` 游标）

- **理由**：私聊已有 `/auth/message/sync`，群聊没有（§3.5）。离线/断线的群成员会永久错过实时推送，只能「重新进群拉全量历史」。补一个 `GET /auth/groups/:id/messages/sync?after_id=`，与私聊能力对齐。
- **影响范围**：`group_dao.go`（加 `SyncGroupMessages(groupID, afterID, limit)`）、`group_message_service.go`（新方法）、`router.go` / `handler`。
- **工作量**：中（半天–1 天），完全复用私聊 `SyncMessages` 的游标语义（`message_service.go:179-218`）。
- **是否值得**：**值得**。这是「消息不丢」主线里唯一一处真实缺口，且实现成本低。

### ③ RabbitMQ publisher confirm + 异步日志上下文

- **理由**：publisher confirm 让 outbox 的 `published` 从「套接字写成功」升级为「broker 持久化确认」（§3.2），堵住实时推送静默丢失的最后一个洞；异步日志带上 `request_id/message_id/group_id/instance_id`（§5.2），让线上能定位到具体消息与用户。
- **影响范围**：`pkg/core/rabbitmq.go`（开 confirm + `WaitForConfirms`）、`group_message_publisher.go`、`group_message_consumer.go` / `outbox_service.go` / `ws_hub.go` 的日志打点。
- **工作量**：小–中（1–2 天）。confirm 是几行改动；日志上下文需要把结构化字段贯穿几条异步路径。
- **是否值得**：**值得**。可观测性是最容易被低估、但线上出问题最先救命的一项。

---

### 附：不做（或暂缓）的清单（避免过度设计）

- **16 分片群在线缓存 + 每条消息 16 goroutine**（`group_cache_service.go`、`group_message_consumer.go:264-297`）：当前单实例/中小群规模下收益被复杂度抵消，属「为面试/为未来大群而设计」。保留可，但别再投入优化。
- **cache-aside + 自旋锁 + 空值缓存的用户列表缓存**（`cache_support.go`、`user_service.go:175-235`）：对低价值数据（用户列表/单用户资料）投入了过重的防击穿机制。收益有限，不改。
- **多实例踢人广播**（§2.5）：单实例无需，多实例上线前再做。
- **分布式追踪（OTel/Jaeger）**：当前规模过重，先做 §6③ 的日志上下文即可。
- **业务错误码枚举**：接入告警/前端强分支前不必引入。

### 文档滞后（顺手清理，非代码）

- `docs/openapi.yaml`：仍含 Post/Comment/Interaction/Social 全套路径（`:311-625`），与「功能收敛为纯实时 IM」的现状（README、`000002_drop_community`）矛盾。
- `pkg/core/migrations/000001_init.up.sql` 建了 posts/comments/likes/…（`:17-114`），`000002` 又删（`000002_drop_community.up.sql:5-11`）——新库白跑一遍 DDL，可把社区表从 000001 移除或保留（不改历史迁移是当前约定，仅提示）。

---

*报告结束。所有「真问题」均已给 `文件:行号`；无证据处已标注「推测」。*
