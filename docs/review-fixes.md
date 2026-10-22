# 评审问题修复清单（对应线上实测报告）

> 范围：`DI`（Go 后端）+ `DI-Web`（Vue 前端）。
> 所有改动均已在本机真实链路验证：MySQL/Redis/RabbitMQ（docker compose）+ 真实后端进程 +
> 真实 Vite 页面（Headless Chrome/CDP，dev 与生产构建各跑一轮）。
> ⚠ 线上服务器需要按 [deploy-frontend-nginx.md](deploy-frontend-nginx.md) 重新发布二进制与前端产物后生效。

## 修复总览

| # | 优先级 | 评审问题 | 修复方式 | 状态 |
| --- | --- | --- | --- | --- |
| 1 | P0 | 新账号登录是空应用，无法当面试 Demo | 新增 `cmd/seed` 幂等种子数据（demo/alice/bob/carol + 1 群 + 12 条群聊 + 10 条私聊 + 未读）；登录页新增「体验 Demo」免密入口（`POST /user/demo-login`，`demo.enabled` 控制） | ✅ 本机验证：Demo 一键进入，3 个会话/12 条群消息 |
| 2 | P0 | `GET /user/search` 未鉴权，可枚举用户与在线状态 | 用户搜索/在线列表/资料/在线状态全部移到 `/auth` 下，必须 JWT；公开路由只留健康检查、注册登录、Demo 入口、WS | ✅ 未登录 404/401，登录后 200 |
| 3 | P1 | 群消息发送者显示「用户13」，刷新前后不一致 | 后端 `GetGroupMessages` / `SendGroupMessage` 返回 DTO 直接带 `username`；MQ 事件与 WS 推送也带 `username` | ✅ 历史消息发送者 alice/bob/carol/demo，无「用户{id}」 |
| 4 | P1 | 群详情串行 N+1，约 2.2s | `GetGroupMembers` / `GetGroupMessages` 批量查 username；前端 `Promise.all` 并行 group/members/messages，删除逐用户补查；加载状态改为骨架屏 | ✅ 400ms 模拟 RTT：3 个 API 在 1ms 内并发，0 个 `/user/{id}`，首屏 787ms |
| 5 | P1 | 群卡片只能鼠标点击 | 改为原生 `<RouterLink>`，补 `aria-label` 与 `:focus-visible` 焦点环 | ✅ Tab 聚焦 + Enter 进入群详情 |
| 6 | P1 | 解散群仍出现在「我的群组」 | `ListGroupsByUserID` 只查 `status=normal`，service 二次过滤；群详情对 dissolved 返回 **410 Gone**；前端 normal/已解散分区 | ✅ 解散后列表消失、详情 410 |
| 7 | P1 | 退出登录不吊销 JWT | 新增 `TokenRevocationStore`（Redis）：登出把 token 指纹加黑名单（TTL 到过期）；改密/角色变更写 user 级 `invalid_before`，旧 token 全部失效；HTTP 与 WS 握手都校验 | ✅ logout 后同一 token 立即 401；WS 握手被拒 |
| 8 | P1 | 注册无验证码/可连续批量创建 | 注册限流收紧为「每 IP 20 秒 1 次」（第二个请求 429）；新增可选邀请码 `auth.register_invite_code`（前端自动显示输入框）；注册成功/失败写审计日志 | ✅ 连续注册第二个 429；邀请码缺失 403；配置后前端显示邀请码 |
| 9 | P2 | 移动端设置图标只有一个小圆点 | 侧栏与 tabbar 共用同一份 ICONS，设置使用完整齿轮 path | ✅ path 长度 597，图标完整 |
| 10 | P2 | 成员操作按钮挤压长用户名 | 用户名 `min-width:0 + text-overflow:ellipsis`；三个单字按钮改为「⋯」菜单（转让群主/设为管理员/取消管理员/移出群聊，含 aria-label） | ✅ 长用户名省略号、菜单可读 |
| 11 | P2 | 登录页回车不提交 | `<form @submit.prevent="onSubmit">` + 按钮 `type="submit"`，去掉 `type="button"` | ✅ 真实 Enter 键触发登录 |
| 12 | P2 | 登录页对比度不足 | 提高 label/提示/占位符/页脚文字对比度（≥4.5:1 方向），加深链接色 | ✅ 视觉复核 |
| 13 | P2 | 移动端 44px 点击区 | 登录页、tabbar、发送/返回/关闭/成员操作菜单、搜索按钮、设置页等统一 ≥44px | ✅ 390px 下 tab 高度 54px、无横向溢出 |
| 14 | P2 | `/api` 301 到 HTTP | 后端同时注册 `/auth/groups` 与 `/auth/groups/` 消除 307；Nginx 增加 `absolute_redirect off`、`location = /api` 308 相对跳转、`proxy_redirect off`，并提供 HTTPS 模板 | ✅ 本机接口无重定向；配置已加固 |
| 15 | P1 | 评审遗留 `review_*` 测试账号/解散群 | `cmd/seed` 提供 `-purge-users-prefix` / `-purge-dissolved-groups`（需 `-yes` 确认） | ✅ 本机清理验证（线上发布脚本会自动清理并先备份 DB） |
| 16 | P1 | 群成员列表仍包含被踢/已退群成员 | `GetGroupMembers` 只返回 active 成员；踢人/退群/审批通过后立即刷新群 active 成员缓存（`RefreshActiveMembers`），被踢成员不再收到群消息 | ✅ 扩展 E2E：踢人后成员数 4→2、被踢成员 5s 内无群消息推送 |
| 17 | P1 | 改密后「同一秒内签发」的旧 token 漏吊销 | JWT 增加毫秒级 `iat_ms` 声明；`invalid_before` 改存毫秒并兼容旧秒级数据；HTTP/WS 均按毫秒判断 | ✅ 扩展 E2E：改密后旧 token 立即 401、新密码可登录 |
| 18 | P1 | 手机端点进私聊先显示「...」/「还没有消息」空态，3–4s 后才正常 | 会话列表/搜索结果的用户名写入 Pinia 缓存，进聊天立即渲染头部；`messageList` 返回的 `peer_user` 直接复用（少发一次 `/user/{id}`）；点击会话先预取历史（单一请求复用）；加载期显示骨架气泡而不是空态；手机端聊天区按 `100dvh - 顶栏 - tabbar` 占满高度，不再只显示半屏卡片 | ✅ 新增 `chat-entry-check.mjs`：8/8（缓存的用户名、骨架屏、无空态闪烁、单请求、占满高度、400ms RTT 下首屏 240ms） |

## 安全相关额外修复

- JWT 中间件与 WS 握手都校验签名算法为 HMAC，防算法混淆
- `GET /auth/user/{id}` 改为脱敏 DTO（只返回 id/username/role/created_at）
- 登录失败统一返回「用户名或密码错误」，避免用户名枚举
- 注册成功/失败、Demo 登录、token 吊销失败均有结构化日志
- 客户端 401 统一清理本地凭证并回登录页；token 过期/损坏在路由守卫直接拦截

## 未覆盖 / 后续可选

- **refresh token / 缩短 TTL**：本次用「服务端主动吊销」解决 logout 失效；要做无感续期需引入 refresh token 流程（工作量大，未在本次范围）
- **图形验证码/Turnstile**：本次用「严格 IP 限流 + 邀请码 + 审计日志」；接验证码需外部服务与密钥，演示环境邀请码已足够
- **只读 Demo 账号**：当前 Demo 账号可发消息（方便演示交互）；如果面试时不希望访客写数据，可给 demo 用户加 role 限制
- 线上实际耗时仍受服务器网络/2C2G 性能影响；本次只消除了串行与 N+1，接口本身的 900ms 若是公网 RTT 需结合 CDN/机房优化

## 本机验证命令

```bash
# 后端
cd DI
go vet ./... && go test ./...
go build ./... && go run ./cmd/seed

# 前端
cd ../DI-Web
npm run build && npm run lint
npm run preview   # 生产产物 + 代理预览

# 冒烟（可选，需要本机已启动 Chrome --remote-debugging-port=9223）
node scripts/verify/ui-smoke.mjs          # 19 项 UI 检查
node scripts/verify/group-load-check.mjs  # 群详情并行/骨架屏/N+1（400ms RTT 模拟）
node scripts/verify/chat-entry-check.mjs  # 手机端私聊首屏：缓存用户名/骨架屏/单请求/占满高度
```

最近一轮验证结果（2026-10-22）：

- `go vet ./...` / `go test ./...` 通过（新增 token 吊销毫秒精度、JWT 中间件单测）
- 扩展 E2E：**15/15**（私聊实时 + 已读回执 + sync、建群/设管/踢人/转让/解散、
  被踢成员不再收群消息、改密旧 token 401）
- 快速回归：**11/11**（未鉴权 404、Demo 数据、群消息 username、登出吊销、注册限流）
- UI 冒烟：**19/19**（dev 与生产构建各跑一轮）
- 手机端私聊首屏：**8/8**（dev 与生产构建各跑一轮；400ms RTT 模拟下 240ms 出消息）

## 发布到线上（服务器已定位：AliYunECS / di.syloe1.xyz）

线上服务器 `8.152.163.211:22345`（用户 `wk`，Cloudflare Tunnel 终止 TLS）已确认：
当前 running 的是 **9 月 24 日的旧二进制**，因此评审报告里的问题都是旧版本行为。

发布包已构建并上传到服务器（`wk` 权限即可）：

```text
/tmp/di-deploy/
├── go-admin              # 新后端（linux/amd64，含全部修复）
├── seed                  # 演示数据工具
├── di-web-dist.tar.gz    # 新前端构建产物
├── config.yaml           # 新版配置（含 auth / demo 段）
├── nginx-di-web.conf     # 适配 Cloudflare Tunnel 的站点配置（已加 301/308 加固）
└── remote-deploy.sh      # 一键发布脚本（备份 + 回滚 + 验收）
```

**只差最后一步（需要 sudo 密码）**——在任意终端执行：

```bash
ssh AliYunECS 'sudo bash /tmp/di-deploy/remote-deploy.sh'
```

脚本会：备份二进制/配置/env/nginx/前端 → 安装新版 → 重启并等待就绪 →
执行 seed（幂等）→ 清理 `review_*` 评审账号（先 mysqldump 备份）→ 替换前端 →
nginx -t 后 reload → 跑完整验收（readyz / register-config / 未鉴权 404 / Demo 登录 /
登出后旧 token 401），失败自动回滚。

当前部署包 SHA256：`BF464A9D664DE447EA9F41FFCABBDA50200565984B26CB66646144271D01C87C`
（2026-10-22 18:39 更新：前端包含移动端私聊首屏体验修复，后端二进制不变）

也可以不用脚本，手动发布步骤：

1. 后端：`./deploy/build-native.sh root@<server>`（或 `SEED=1 ./deploy/build-native.sh ...` 自动写演示数据）
2. 前端：`npm run build` → 上传 `dist` 到 `/var/www/di-web` → reload nginx
3. 环境变量（`/opt/go-admin/go-admin.env`）建议：
   - `APP_AUTH_REGISTER_INVITE_CODE=<随机邀请码>`
   - `APP_DEMO_ENABLED=true`
   > ⚠ 同时确认服务器 `/opt/go-admin/config/config.yaml` 里有 `auth` / `demo` 段
   > （从新 `config/config.example.yaml` 复制，或手动补上）。viper 对
   > 配置文件里未声明的键，环境变量会**静默失效**，表现就是邀请码/Demo 入口不生效。
4. 服务器执行一次 seed：
   `set -a; . /opt/go-admin/go-admin.env; set +a; /opt/go-admin/seed -config /opt/go-admin/config/config.yaml`
5. 验证：`/api/user/register/config` 返回 `invite_required=true`、`demo_enabled=true`；
   登录页出现「体验 Demo」；登出后旧 token 调 `/api/auth/message/conversations` 返回 401
