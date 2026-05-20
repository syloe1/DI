# 生产级高并发聊天系统架构设计

## 1. 设计目标

本设计面向生产级实时聊天系统，核心目标是：

- 使用 QUIC 承载长连接通信，降低弱网和移动网络下的连接迁移成本。
- 使用 RabbitMQ fanout 完成群聊消息广播事件分发。
- 使用 Redis 存储在线状态、连接路由、群成员缓存和热点数据。
- 使用 MySQL 作为消息记录、群组、成员关系等强一致持久化存储。
- 支持多实例水平扩展。
- 解决当前系统存在的 MQ 单消费协程、群成员无缓存、长连接无心跳、推送性能低、无状态同步等问题。

## 2. 总体架构图描述

```text
                         +----------------------+
                         |      Client App      |
                         |  QUIC Long Conn      |
                         +----------+-----------+
                                    |
                                    | QUIC
                                    v
        +---------------------------+---------------------------+
        |                       LB / Gateway                    |
        |              QUIC 负载均衡 / 四层转发                 |
        +-------------+--------------------------+--------------+
                      |                          |
                      v                          v
        +-------------+------------+   +---------+----------------+
        | Chat Gateway Instance A  |   | Chat Gateway Instance B |
        | QUIC Session Manager     |   | QUIC Session Manager    |
        | Push Worker Pool         |   | Push Worker Pool        |
        +-------------+------------+   +---------+----------------+
                      |                          |
                      +------------+-------------+
                                   |
                                   v
                         +---------+----------+
                         |       Redis        |
                         | online:{uid}       |
                         | route:{uid}        |
                         | group_members:{gid}|
                         | group_online:{gid} |
                         +---------+----------+
                                   |
              +--------------------+--------------------+
              |                                         |
              v                                         v
    +---------+----------+                    +---------+----------+
    |     MySQL          |                    |     RabbitMQ       |
    | messages           |                    | fanout exchange    |
    | groups             |                    | group.msg.exchange |
    | group_members      |                    | per-instance queue |
    +--------------------+                    +---------+----------+
                                                        |
                                                        v
                                   +--------------------+--------------------+
                                   |                                         |
                                   v                                         v
                         +---------+----------+                    +---------+----------+
                         | Consumer Pool A    |                    | Consumer Pool B    |
                         | Local Push Dispatch|                    | Local Push Dispatch|
                         +--------------------+                    +--------------------+
```

关键点：

- QUIC Gateway 多实例部署，每个实例只维护本机 QUIC 连接。
- Redis 存储用户在线状态和用户所在实例路由，解决多实例之间状态不可见的问题。
- RabbitMQ fanout exchange 将群消息事件广播给所有 Chat Gateway 实例。
- 每个实例使用独立队列绑定 fanout exchange，只推送本机在线用户。
- MySQL 只负责消息最终持久化，不承担在线状态和高频广播状态查询。

## 3. 核心模块职责

### 3.1 QUIC Gateway

职责：

- 接收客户端 QUIC 长连接。
- 完成登录态校验、连接注册、连接注销。
- 维护本机 `uid -> session` 映射。
- 接收客户端上行消息。
- 执行基础协议解析、限流、参数校验。
- 将消息转交 Chat Service 处理。
- 接收本实例 Consumer 的推送任务并写回客户端。

生产要求：

- 每个连接必须有读超时、写超时、心跳超时。
- 每个连接有独立 send queue，队列满时执行降级策略。
- 写操作必须由固定写协程负责，避免多 goroutine 并发写同一个 QUIC stream。
- 支持优雅下线，实例退出前从 Redis 移除路由并拒绝新连接。

### 3.2 Session Manager

职责：

- 管理本机在线连接。
- 维护本机内存表：

```text
local_sessions: uid -> Session
```

- 连接建立时写 Redis：

```text
online:{uid} = 1
route:{uid} = instance_id
```

- 连接断开时清理 Redis。
- 定期续租在线状态 TTL。

建议 Redis 结构：

```text
online:{uid}                 string, TTL
route:{uid}                  string, TTL, value = instance_id
instance:{instance_id}:users set, TTL
group_online:{gid}           set, members = uid
```

### 3.3 Chat Service

职责：

- 处理消息业务逻辑。
- 校验用户是否为 active 群成员。
- 校验群状态是否 normal。
- 消息写入 MySQL。
- 写入成功后发布 RabbitMQ 事件。

原则：

- MySQL 入库是主流程。
- RabbitMQ 发布是广播事件，不替代消息存储。
- 入库成功但 MQ 发布失败时，需要有补偿机制，例如 outbox 表。

### 3.4 RabbitMQ Publisher

职责：

- 发布 `group_message_created` 事件到 fanout exchange。
- 生产者 channel 与消费者 channel 分离。
- 发布失败时记录错误，并交给 outbox retry。

事件结构：

```json
{
  "type": "group_message_created",
  "message_id": 10001,
  "group_id": 20001,
  "from_uid": 30001,
  "content": "hello",
  "created_at": "2026-05-19T10:00:00+08:00"
}
```

### 3.5 RabbitMQ Consumer Pool

职责：

- 每个 Chat Gateway 实例启动自己的 consumer queue。
- 每个实例队列绑定同一个 fanout exchange。
- 每个实例只推送本机在线用户。
- 使用 worker pool 并发处理事件，避免单协程消费瓶颈。

建议：

```text
consumer_workers = CPU * 2 ~ CPU * 8
prefetch = consumer_workers * 2
```

### 3.6 Redis Cache

职责：

- 在线状态。
- 用户连接路由。
- 群成员缓存。
- 群在线成员缓存。
- 热点群基础信息缓存。

群成员缓存建议：

```text
group_members:{gid} set(uid...)
group_member_status:{gid}:{uid} string(active/left/kicked)
group_version:{gid} int
```

成员变更时：

- 先更新 MySQL。
- 再删除或刷新 Redis 群成员缓存。
- 使用版本号避免旧缓存覆盖新状态。

### 3.7 MySQL Storage

职责：

- 消息最终存储。
- 群信息存储。
- 群成员关系存储。
- 离线消息拉取。
- 消息审计和回溯。

核心表：

```text
chat_group_messages
chat_groups
chat_group_members
message_outbox
```

推荐索引：

```sql
chat_group_messages:
  idx_group_created_at(group_id, created_at)
  idx_group_id_id(group_id, id)
  idx_sender_created_at(sender_uid, created_at)

chat_group_members:
  uniq_group_user(group_id, user_id)
  idx_group_status(group_id, status)
  idx_user_status(user_id, status)

message_outbox:
  idx_status_next_retry(status, next_retry_at)
```

## 4. 消息流转流程

### 4.1 客户端发送群消息

```text
Client
  -> QUIC Gateway
  -> Chat Service
  -> Redis 校验或 MySQL 回源
  -> MySQL 写入消息
  -> RabbitMQ fanout 发布事件
  -> 返回发送成功 ACK
```

详细流程：

1. 客户端通过 QUIC 发送：

```json
{
  "type": "group_message",
  "request_id": "client-msg-uuid",
  "group_id": 1,
  "content": "hello"
}
```

2. Gateway 校验连接身份。
3. Chat Service 校验：

```text
group.status == normal
member.status == active
content 非空且长度合法
```

4. 消息写入 MySQL。
5. 写入成功后发布 RabbitMQ 事件。
6. 服务端返回 ACK：

```json
{
  "type": "group_message_ack",
  "request_id": "client-msg-uuid",
  "message_id": 10001,
  "status": "stored"
}
```

### 4.2 RabbitMQ 广播

```text
RabbitMQ fanout exchange
  -> instance_a_queue
  -> instance_b_queue
  -> instance_c_queue
```

每个在线实例都会收到事件，但每个实例只负责推送自己本机连接的用户。

### 4.3 Consumer 推送在线群成员

```text
Consumer Worker
  -> 获取 group_online:{gid}
  -> 与本机 local_sessions 求交集
  -> 投递到每个 Session 的 send queue
  -> QUIC 写协程推送给客户端
```

推送消息：

```json
{
  "type": "group_message",
  "message_id": 10001,
  "group_id": 1,
  "from_uid": 30001,
  "content": "hello",
  "created_at": "2026-05-19T10:00:00+08:00"
}
```

### 4.4 离线用户拉取消息

离线用户不走实时推送。

用户重新上线后：

1. 客户端携带 last_message_id。
2. 服务端查询 MySQL：

```sql
SELECT * FROM chat_group_messages
WHERE group_id = ?
AND id > ?
ORDER BY id ASC
LIMIT ?
```

3. 返回离线期间遗漏消息。

## 5. 解决大群推送瓶颈

当前大群瓶颈通常来自：

- 每条消息都查 MySQL 群成员。
- 每条消息对全量成员循环推送。
- 单 consumer 串行消费。
- 单实例承载过多在线连接。

生产级策略：

### 5.1 只推在线成员

不要对群全量成员推送，只对在线成员推送。

Redis 维护：

```text
group_online:{gid} set(uid...)
```

用户上线：

- 查询用户 active 群列表。
- 将 uid 加入这些群的 `group_online:{gid}`。

用户下线：

- 从相关 `group_online:{gid}` 移除 uid。

### 5.2 本机实例只推本机连接

consumer 收到 fanout 事件后：

```text
target_online_users = Redis.SMEMBERS(group_online:{gid})
local_users = local_sessions.keys()
push_users = intersection(target_online_users, local_users)
```

这样不会发生每个实例都尝试推所有人的问题。

### 5.3 推送 worker pool

consumer 不直接逐个写连接，而是投递任务到 worker pool：

```text
RabbitMQ Consumer
  -> dispatch task
  -> push_worker_1
  -> push_worker_2
  -> push_worker_N
```

每个 worker 批量处理用户列表，减少单 goroutine 阻塞。

### 5.4 分片大群

对超大群可以按 uid hash 分片：

```text
group_push_shard:{gid}:0
group_push_shard:{gid}:1
...
group_push_shard:{gid}:N
```

RabbitMQ 仍使用 fanout 传播事件，每个实例内部按 shard 并行推送。

### 5.5 慢连接降级

每个连接 send queue 设置容量，例如 256 或 1024。

队列满时策略：

```text
普通消息：丢弃实时推送，让客户端稍后从 MySQL 补拉
重要消息：断开慢连接，要求客户端重连并补拉
系统消息：高优先级队列
```

## 6. 多实例部署设计

### 6.1 实例注册

每个实例启动时生成：

```text
instance_id = hostname + process_id + random_id
```

写入 Redis：

```text
instance:{instance_id}:heartbeat = timestamp
```

并定期续租。

### 6.2 用户连接路由

用户连接到某个实例后：

```text
route:{uid} = instance_id
online:{uid} = 1
```

如果同一用户多端登录：

```text
route:{uid}:{device_id} = instance_id
online_devices:{uid} set(device_id)
```

### 6.3 实例队列

每个实例声明独立队列：

```text
queue = group_message_push.{instance_id}
exchange = group_message_exchange
type = fanout
```

实例关闭时可以删除临时队列，或使用 TTL 自动过期。

### 6.4 故障处理

实例异常退出时：

- Redis 在线状态通过 TTL 自动过期。
- RabbitMQ 队列可以设置 exclusive/auto-delete，避免废弃队列堆积。
- 客户端 QUIC 断线后自动重连。
- 客户端根据 last_message_id 从 MySQL 补拉遗漏消息。

## 7. 针对当前问题的改造方案

### 7.1 MQ 单消费协程

当前问题：

```text
一个 consumer goroutine 串行处理所有群消息事件。
```

改造：

```text
RabbitMQ Consume
  -> delivery channel
  -> worker pool
  -> 每个 worker 并发处理消息
```

关键点：

- 设置 `Qos(prefetch)`。
- worker 数可配置。
- 单条消息处理失败时 Nack。
- 对不可恢复错误不要无限 requeue，避免死循环。

### 7.2 群成员无缓存

当前问题：

```text
每次发送或消费都查 MySQL 群成员。
```

改造：

- `group_members:{gid}` 缓存 active 成员。
- `group_online:{gid}` 缓存在线成员。
- 成员变更时删除缓存或发布缓存失效事件。
- 缓存 miss 时回源 MySQL。

### 7.3 长连接无心跳

当前问题：

```text
连接断开可能无法及时发现，Redis 在线状态不准确。
```

改造：

- 客户端每 15-30 秒发送 ping。
- 服务端返回 pong。
- 服务端维护 last_active_at。
- 超过 60-90 秒无心跳则关闭连接。
- Redis 在线状态 TTL 每次心跳续租。

### 7.4 推送性能低

当前问题：

```text
consumer 同步循环推送，慢连接会拖慢整体。
```

改造：

- consumer 只负责分发任务。
- 推送由 push worker pool 处理。
- 每个连接有独立 send queue。
- 写协程只负责写当前连接。
- 队列满时降级。

### 7.5 无状态同步

当前问题：

```text
每个实例只知道本机连接，多实例之间不知道用户在哪。
```

改造：

- Redis 存储 `route:{uid}`。
- Redis 存储 `instance:{id}:users`。
- RabbitMQ fanout 将事件广播给所有实例。
- 每个实例收到事件后只推本机在线用户。

## 8. 高并发优化策略

### 8.1 数据库优化

- 消息表按时间或 group_id 分区。
- 使用递增 message_id，便于分页和补拉。
- 热点查询只查索引覆盖字段。
- 群消息写入采用单表先行，达到瓶颈后再分库分表。
- 增加 outbox 表，解决消息入库成功但 MQ 发布失败的问题。

### 8.2 RabbitMQ 优化

- producer channel 和 consumer channel 分离。
- 每个实例使用独立 queue。
- consumer 使用 worker pool。
- 配置 prefetch。
- 对失败消息设置重试次数和死信队列。
- 大流量下可按业务拆 exchange，例如普通消息、系统消息、控制事件。

### 8.3 Redis 优化

- 在线状态使用 TTL。
- 群成员缓存使用 set。
- 大群在线成员可分片 set。
- 热点群缓存本地 LRU + Redis 二级缓存。
- 缓存失效使用版本号，避免并发覆盖。

### 8.4 QUIC 长连接优化

- 控制最大连接数。
- 心跳保活。
- 慢连接检测。
- send queue 背压。
- 单连接单写协程。
- 连接迁移时刷新 Redis route。

### 8.5 服务端限流

维度：

```text
user_id
group_id
ip
device_id
```

策略：

- 单用户发消息限流。
- 单群写入限流。
- 大群突发广播限流。
- 恶意连接限流。

### 8.6 可观测性

必须监控：

- 当前 QUIC 连接数。
- 每实例在线用户数。
- RabbitMQ queue backlog。
- RabbitMQ publish latency。
- consumer 处理耗时。
- MySQL 慢查询。
- Redis QPS 和延迟。
- 每个连接 send queue 长度。
- 消息端到端延迟。

## 9. 关键代码结构

只列结构，不写具体实现细节。

```text
cmd/
  chat-server/
    main.go

config/
  config.go
  config.yaml

internal/
  gateway/
    quic_server.go
    quic_session.go
    session_manager.go
    heartbeat.go
    push_dispatcher.go
    push_worker_pool.go

  protocol/
    frame.go
    codec.go
    message_type.go

  service/
    chat_service.go
    group_message_service.go
    offline_message_service.go

  mq/
    rabbitmq.go
    publisher.go
    consumer.go
    consumer_pool.go
    event.go

  cache/
    online_cache.go
    route_cache.go
    group_member_cache.go
    group_online_cache.go

  dao/
    message_dao.go
    group_dao.go
    group_member_dao.go
    outbox_dao.go

  model/
    message.go
    group.go
    group_member.go
    outbox.go

  job/
    outbox_retry_job.go
    instance_heartbeat_job.go
    stale_connection_cleaner.go

pkg/
  quic/
    server.go
    connection.go

  idgen/
    snowflake.go

  observability/
    metrics.go
    tracing.go
```

## 10. 推荐落地顺序

### 第一阶段：稳定单实例

1. 增加 QUIC Gateway 基础连接管理。
2. 增加心跳和连接超时。
3. 每个连接增加 send queue。
4. RabbitMQ producer/consumer channel 分离。
5. consumer 改 worker pool。

### 第二阶段：多实例可用

1. Redis 增加在线状态和 route。
2. RabbitMQ fanout 改为每实例独立 queue。
3. consumer 只推本机在线用户。
4. 支持实例优雅上下线。

### 第三阶段：大群优化

1. Redis 缓存群 active 成员。
2. Redis 缓存群在线成员。
3. 大群在线成员分片。
4. 推送 worker pool 分片并行。
5. 慢连接降级。

### 第四阶段：生产可靠性

1. 增加 outbox 可靠事件发布。
2. 增加死信队列。
3. 增加压测和容量评估。
4. 增加监控告警。
5. 增加灰度发布和故障演练。

## 11. 容量预估方法

不要只用理论值判断容量，必须压测。

建议压测指标：

```text
在线连接数：1w / 5w / 10w
群消息写入：100/s / 500/s / 1000/s
群大小：100 / 1000 / 10000
在线率：10% / 30% / 80%
消息端到端延迟 P50/P95/P99
RabbitMQ backlog
MySQL TPS
Redis QPS
CPU / 内存 / 网络带宽
```

核心公式：

```text
实时推送次数/秒 = 群消息数/秒 * 群在线人数
```

例如：

```text
100 条群消息/秒 * 每群 1000 在线成员 = 100000 次推送/秒
```

因此大群优化的关键不是“消息写入”，而是“在线成员过滤 + 分片并行推送 + 慢连接降级”。

## 12. 最终建议

当前系统要演进为生产级高并发聊天系统，核心不是单点加机器，而是拆清楚职责：

- QUIC Gateway 只管连接和推送。
- MySQL 只管可靠存储。
- RabbitMQ fanout 只管事件广播。
- Redis 只管在线状态、路由和热点缓存。
- consumer pool 只管并发分发。

这样系统才能水平扩展：新增 Chat Gateway 实例时，只需要新增一个实例队列并注册 Redis 路由，就能承载更多长连接和更多推送压力。
