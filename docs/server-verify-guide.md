# go-admin 服务器修复与验证指南（供 Claude 按序执行）

## 给你的任务

你是服务器上的执行助手。本文件是**已经过本地验证**的修复 + 测试流程。
**严格按顺序执行**，任何一步的输出与「预期」不符 → 立即停下，把实际输出**原样报告**给用户。
不要自行换方案、不要改代码 / 配置 / 数据库结构。

---

## 一、已确认的事实（不要重新诊断）

1. `go-admin` 服务已部署在 `/opt/go-admin`，由 systemd 托管，当前 **active**。
2. 中间件全部正常：MySQL（库 `go`）、Redis、RabbitMQ（vhost `di` / 用户 `di`）。
3. `go-admin.env` 已配置：MySQL 账号 `go`、JWT secret、`APP_RABBITMQ_URL=amqp://di:...@127.0.0.1:5672/di`。
4. **问题根因（已定位，不需要再猜）**：`go` 库里只有 `schema_migrations` 表，
   `version=1, dirty=1`。早期 RabbitMQ 未就绪时进程在迁移中途 FATAL 退出，中断迁移留下脏标记；
   golang-migrate 遇到 dirty 会**拒绝继续执行**，所以业务表从未创建。
5. 影响：业务接口 500（`Error 1146 表不存在`）；`/readyz` 返回 200 是正常的——它只探连通性，不查表结构。
6. 迁移 SQL 全部是 `CREATE TABLE IF NOT EXISTS`，且 DSN 已带 `multiStatements=true`——
   **清脏后重跑是安全的**，不会发生「表已存在」冲突。

---

## 二、修复步骤

```bash
# 0. 先确认当前确实没有业务表（安全前提）
mysql -ugo -pqaz123 go -e "SHOW TABLES;"
#    预期：只有 schema_migrations 一张。若已有业务表，停下报告。

# 1. 清脏标记（go 账号有权限，无需 sudo）
#    ⚠ 用 DELETE 清空，不要用 UPDATE ... SET version=0：
#    golang-migrate 对 version=0 没有对应迁移文件，会报
#    "no migration found for version 0" 卡住；空表 = NilVersion = 全新库，Up() 才能重跑。
mysql -ugo -pqaz123 go -e "DELETE FROM schema_migrations;"

# 2. 重启服务（需要 sudo）
sudo systemctl restart go-admin

# 3. 等 3 秒让迁移跑完
sleep 3

# 4. 确认建表结果
mysql -ugo -pqaz123 go -e "SHOW TABLES;"
```

**预期**：出现 **8 张业务表**（另有 schema_migrations）：

```
users
messages
chat_groups
chat_group_members
chat_group_invitations
chat_group_join_requests
chat_group_messages
message_outboxes
```

> ⚠ 注意：是 **8 张业务表**。旧文档里的 posts/comments/likes/dislikes/collects/shares/user_relations
> 已被迁移 000002_drop_community 删除（项目收敛为纯 IM），不要因为看不到它们就认为迁移没跑完。
> 只出现部分表 = 迁移没跑完，停下报告。

---

## 三、验证清单（按序执行，每步贴结果）

### 1. 服务状态 + 日志

```bash
sudo systemctl status go-admin --no-pager | head -20
sudo journalctl -u go-admin -n 40 --no-pager
```

**预期**日志依次出现：`database migration completed` → `DI container initialized` →
`server started, port: 19999`。若出现 `database migration failed`，停下报告。

### 2. 健康检查

```bash
curl -sS http://127.0.0.1:19999/readyz
```

**预期**：`{"code":200,"msg":"success","data":{"status":"ok"}}`

### 3. 注册（往 users 表写数据，表存在才算成功）

```bash
curl -sS -X POST http://127.0.0.1:19999/user/register \
  -H 'Content-Type: application/json' \
  -d '{"username":"probe","password":"probe123456"}'
```

**预期**：`{"code":200,"msg":"register success"}`

> 若返回 `username already exists`（probe 已存在）或限流提示，属正常，换 `probe2` 重试即可。
> 若返回 **500 / Error 1146**，说明迁移没成功 → 走第四节。

### 4. 登录（读 users 表 + 签发 JWT）

```bash
curl -sS -X POST http://127.0.0.1:19999/user/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"probe","password":"probe123456"}'
```

**预期**：返回 JSON 且带 `token` 字段。

### 5. 带 token 访问鉴权接口（证明 JWT 链路通）

```bash
TOKEN='<上一步返回的 token>'
curl -sS http://127.0.0.1:19999/auth/user/list -H "Authorization: Bearer $TOKEN"
```

**预期**：HTTP 200 + JSON（内容可以是空列表），**不能是 500**。

### 6. 公开查询接口（读 users 表，应正常）

```bash
curl -sS -w '\nHTTP %{http_code}\n' http://127.0.0.1:19999/user/online/list
curl -sS -w '\nHTTP %{http_code}\n' 'http://127.0.0.1:19999/user/search?kw=probe'
```

**预期**：HTTP 200（内容可为空列表/空对象），**不能是 500**。

### 7. RabbitMQ 连接确认

```bash
sudo rabbitmqctl list_connections user vhost
```

**预期**：能看到 `di   di   running` 这一行。

---

## 四、如果修复后又变 dirty=1（真失败，不是中断）

**不要改 SQL、不要 DROP 表、不要重试其他命令。** 执行：

```bash
sudo journalctl -u go-admin | grep -iE 'migrat|error|fatal' | tail -40
```

把输出**原样**报告给用户。

---

## 五、收尾

全部通过后，向用户报告：

1. `readyz` 结果
2. `SHOW TABLES` 的 8 张业务表
3. 注册 / 登录 / 带 token 接口的结果
4. 结论：后端部署完成，可以进入前端 Nginx 部署

---

## 附：数据库账号（仅用于只读诊断）

```bash
mysql -ugo -pqaz123 go   # 库：go，账号：go
```
