# Chat Reliability Runbook

## 目标

第四阶段解决的是生产可靠性问题：消息不能因为 MQ 瞬时失败而丢失，坏消息不能拖垮 consumer，高并发下要能观测、压测、灰度和演练。

## Outbox 可靠发布

消息写入 MySQL 时，同一个事务写入 `message_outboxes`。

流程：

1. 业务层校验群状态和成员状态。
2. 写入 `chat_group_messages`。
3. 同事务写入 `message_outboxes`，状态为 `pending`。
4. `OutboxService` 定时扫描到期 `pending` 事件。
5. 发布 RabbitMQ 成功后标记为 `published`。
6. 发布失败后增加 `retry_count`，写入 `last_error`，按退避时间重试。
7. 重试超过阈值后标记为 `failed`，由人工或后台补偿任务处理。

容量关注：

- `pending` 数量持续上升：MQ 或发布链路异常。
- `failed` 数量大于 0：需要排查 `last_error`。
- `outbox` 表需要按 `status,next_retry_at` 建索引，避免扫描全表。

## Dead Letter Queue

每个实例队列都有对应 DLQ：

- 主 exchange：`<exchange>`
- 死信 exchange：`<exchange>.dlx`
- 实例队列：`<queue>.<instance_id>`
- 死信队列：`<queue>.<instance_id>.dlq`

consumer 处理失败时使用 `Nack(false, false)`，消息进入 DLQ。

这样做的目的：

- 避免 poison message 无限重试。
- 保留原始消息，便于排查和人工重放。
- consumer worker pool 不会被同一条坏消息反复占满。

## Metrics

当前暴露地址：

```text
http://127.0.0.1:6060/metrics
```

核心指标：

```text
chat_online_connections
chat_push_delivered_total
chat_push_dropped_slow_total
chat_consumer_succeeded_total
chat_consumer_failed_total
chat_outbox_published_total
chat_outbox_publish_failed_total
```

建议告警：

| 指标 | 告警条件 | 含义 |
| --- | --- | --- |
| `chat_outbox_publish_failed_total` | 5 分钟持续增加 | MQ 发布失败或网络异常 |
| `chat_consumer_failed_total` | 5 分钟持续增加 | consumer 处理异常，检查 DLQ |
| `chat_push_dropped_slow_total` | 1 分钟快速增加 | 慢连接过多或下游网络异常 |
| `chat_online_connections` | 突然下降 30% 以上 | 实例重启、网络闪断或 Redis 异常 |

## 压测方案

压测分三层，不要一开始就压完整链路。

### 1. API 入库压测

目标：验证 MySQL 写入和 outbox 写入能力。

场景：

- 小群：100 个群，每群 20 人。
- 中群：10 个群，每群 1000 人。
- 大群：1 个群，1 万人。

关注：

- 请求成功率。
- P95/P99 延迟。
- MySQL CPU、连接数、慢 SQL。
- `message_outboxes.pending` 堆积量。

### 2. MQ 消费压测

目标：验证 fanout、实例队列和 worker pool。

方法：

- 直接批量写入 outbox 或向 exchange 发布事件。
- 启动 1、2、4 个实例观察吞吐是否近似线性提升。

关注：

- `chat_consumer_succeeded_total` 增长速度。
- `chat_consumer_failed_total` 是否增长。
- 单实例 CPU 是否被 push worker 打满。
- DLQ 是否有消息。

### 3. 长连接推送压测

目标：验证在线连接、send queue、慢连接降级。

方法：

- 准备 QUIC/WS 压测客户端。
- 每个连接登录后保持心跳。
- 模拟部分客户端停止读取，观察慢连接是否被踢。

容量估算：

```text
单实例有效推送 QPS = min(
  consumer_worker吞吐,
  push_worker吞吐,
  网络出口带宽 / 平均消息大小,
  CPU可承载JSON编码和连接写入能力
)
```

例如：

```text
平均消息 1KB，出口带宽 100MB/s，理论网络上限约 100000 msg/s。
如果单实例实测 P99 延迟可接受时为 20000 msg/s，则生产按 50% 安全水位估算为 10000 msg/s。
```

## 灰度发布

推荐步骤：

1. 新版本实例使用新的 `INSTANCE_ID` 启动。
2. 新实例绑定自己的 MQ 队列，不抢旧实例队列。
3. 先导入 1% 用户连接到新实例。
4. 观察 15 分钟：
   - outbox 是否堆积。
   - DLQ 是否增长。
   - 慢连接丢弃是否异常。
   - 用户消息延迟是否升高。
5. 扩到 10%、30%、50%、100%。
6. 出现异常时摘除新实例流量，保留队列和 DLQ 用于排查。

## 故障演练

### RabbitMQ 短暂不可用

操作：

1. 停止 RabbitMQ 30 秒。
2. 持续发送群消息。
3. 恢复 RabbitMQ。

预期：

- 消息已入库。
- outbox 出现 publish failed 计数。
- RabbitMQ 恢复后 outbox 被重新发布。

### consumer 处理坏消息

操作：

1. 手动发布一条非法 JSON 到实例队列。
2. 观察 consumer。

预期：

- `chat_consumer_failed_total` 增加。
- 消息进入 `<queue>.<instance_id>.dlq`。
- worker pool 不被阻塞。

### 慢连接

操作：

1. 构造客户端连接后不读取服务端消息。
2. 大量推送群消息。

预期：

- send queue 满后连接被关闭。
- `chat_push_dropped_slow_total` 增加。
- 其他正常用户不受影响。

### 单实例下线

操作：

1. 停止一个实例。
2. 用户重新连接到其他实例。

预期：

- Redis 在线状态被清理或过期。
- 新实例重新注册在线 route。
- 群消息只推送给本机在线用户。

## 后续增强

当前 outbox 已经能防止“数据库成功但 MQ 发布失败”的丢消息问题。下一步生产增强建议：

1. 给 outbox 增加 `processing` 状态或基于数据库锁的 claim 机制，避免多实例重复扫描同一条 outbox。
2. consumer 做幂等去重，防止发布成功但标记 published 前进程崩溃造成重复推送。
3. 增加 DLQ 重放工具，只允许修复后按 message_id 或时间范围重放。
4. 将 `/metrics` 接入 Prometheus，而不是只本机查看。
