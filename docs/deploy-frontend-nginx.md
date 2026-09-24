# 前端 Nginx 部署 + 整体发布手册（2C2G 裸机）

> 与 [deploy-native.md](deploy-native.md)（后端裸机部署）、[deploy-2c2g.md](deploy-2c2g.md)
> （2C2G 内存调优）配套。本文补齐**前端 Nginx 托管**和**前后端整体发布/验证**流程。
>
> 前提：服务器 Ubuntu 22.04，2C2G，MySQL / Redis / RabbitMQ 已有实例（复用）。

## 1. 发布物（本地已产出）

| 文件 | 来源 | 说明 |
| --- | --- | --- |
| `dist/go-admin` | `go build` 交叉编译（CGO_ENABLED=0, linux/amd64） | 纯静态后端二进制，服务器无需装 Go |
| `dist/di-web-dist.tar.gz` | `npm run build` 后打包 | 前端构建产物（`index.html` + `assets/`） |
| `deploy/nginx/di-web.conf` | 本仓库 | 生产 Nginx 站点配置（含 /api、/ws 代理） |

## 2. 服务器内存急救（先做，2 分钟）

2C2G 现在没内存，多半是 RabbitMQ 默认配置吃满。**先加 swap 兜底，再限中间件内存**：

```bash
# ① swap（立即可用，防 OOM Killer）
fallocate -l 2G /swapfile && chmod 600 /swapfile
mkswap /swapfile && swapon /swapfile
echo '/swapfile none swap sw 0 0' >> /etc/fstab
free -m

# ② RabbitMQ 限内存（最大内存黑洞）+ 关用不上的管理面板
cat >> /etc/rabbitmq/rabbitmq.conf <<'EOF'
vm_memory_high_watermark.relative = 0.35
EOF
rabbitmq-plugins disable rabbitmq_management
systemctl restart rabbitmq-server

# ③ MySQL 瘦身
cat > /etc/mysql/conf.d/2g.cnf <<'EOF'
[mysqld]
innodb_buffer_pool_size = 64M
performance_schema = OFF
max_connections = 50
EOF
systemctl restart mysql
```

> 调优详情与内存账本见 [deploy-2c2g.md](deploy-2c2g.md)。调优后中间件合计约 1.0–1.15GB，
> 应用约 150MB，留 ~700MB 余量。

## 3. 后端一次性准备（首次）

按 [deploy-native.md](deploy-native.md) 第 2 节执行，摘要：

```bash
useradd --system --no-create-home --shell /usr/sbin/nologin go-admin
mkdir -p /opt/go-admin/config
mysql -uroot -p -e "CREATE DATABASE IF NOT EXISTS \`go\` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci;"

cp config/config.example.yaml /opt/go-admin/config/config.yaml   # 只放非敏感项，键一个都别删
cp deploy/go-admin.env.example /opt/go-admin/go-admin.env        # 填 APP_MYSQL_PASSWORD、APP_JWT_SECRET
chmod 600 /opt/go-admin/go-admin.env
cp deploy/systemd/go-admin.service /etc/systemd/system/
systemctl daemon-reload && systemctl enable go-admin
```

> ⚠ viper 陷阱：`config.yaml` 里没声明的键，`APP_*` 环境变量设了也**静默失效**。
> 必须从 `config.example.yaml` 复制，且不删键。详见 deploy-native.md 第 5 节。

## 4. 前端 Nginx（一次性准备）

```bash
apt-get install -y nginx
mkdir -p /var/www/di-web
tar -xzf di-web-dist.tar.gz -C /var/www/di-web        # 解出 dist/ 目录
cp deploy/nginx/di-web.conf /etc/nginx/sites-available/di-web
ln -s /etc/nginx/sites-available/di-web /etc/nginx/sites-enabled/
rm -f /etc/nginx/sites-enabled/default
nginx -t && systemctl reload nginx
```

Nginx 行为与 vite 开发代理一致（[vite.config.ts](../../DI-Web/vite.config.ts)）：

- `/api/*` → 去掉 `/api` 前缀转发到 `127.0.0.1:19999`（同源代理，不触发 CORS）
- `/ws` → WebSocket 升级转发到后端
- 前端静态资源带 hash 长缓存 30 天；SPA 路由回退 `index.html`

> 已在本机用真实 nginx 容器 + 真实构建产物验证过这套配置：首页、SPA 回退、
> `/api/healthz`、`/api/user/login`、WebSocket 升级全部通过。

## 5. 日常发布（每次改代码后）

本地（Windows PowerShell，在仓库根目录）：

```powershell
# 后端：交叉编译 → 上传（先传 .new 再 mv，避免覆盖运行中二进制的 ETXTBSY）
$env:CGO_ENABLED='0'; $env:GOOS='linux'; $env:GOARCH='amd64'
go build -trimpath -o dist/go-admin ./cmd/server
scp dist/go-admin root@<server>:/opt/go-admin/go-admin.new
ssh root@<server> "chmod 755 /opt/go-admin/go-admin.new && mv /opt/go-admin/go-admin.new /opt/go-admin/go-admin && systemctl restart go-admin"

# 前端：构建 → 打包 → 上传 → 解压 → reload
cd ..\DI-Web
npm run build
tar -czf ..\DI\dist\di-web-dist.tar.gz dist
scp ..\DI\dist\di-web-dist.tar.gz root@<server>:/tmp/
ssh root@<server> "tar -xzf /tmp/di-web-dist.tar.gz -C /var/www/di-web && rm /tmp/di-web-dist.tar.gz && systemctl reload nginx"
```

> 后端是二进制替换，前端是静态文件替换，两者互相独立、互不影响回滚。

## 6. 部署后验证

```bash
# 后端
systemctl status go-admin --no-pager
journalctl -u go-admin -n 50 --no-pager      # 应见: server started, port: 19999
curl -sS http://127.0.0.1:19999/readyz       # {"status":"ok"} = MySQL+Redis 都通

# 前端 + 代理（服务器本机或公网 IP）
curl -sS http://127.0.0.1/                   # index.html
curl -sS http://127.0.0.1/api/healthz        # {"code":200,...,"status":"ok"}
curl -sS -X POST http://127.0.0.1/api/user/login -H 'Content-Type: application/json' \
  -d '{"username":"<测试用户>","password":"<密码>"}'   # 返回 token = 全链路通
```

## 7. 安全组 / 防火墙

- 放行 **80**（前端入口）和 **19999**（后端，如需直连调试）
- **6060**（pprof/metrics）已硬编码监听 127.0.0.1，不要放行；远程看用
  `ssh -L 6060:127.0.0.1:6060 root@<server>`

## 8. 排错速查

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| 前端能打开但接口 502 | nginx 连不上后端 | `ss -lntp \| grep 19999` 确认进程在；`curl 127.0.0.1:19999/healthz` 直连对比 |
| 接口 401/403 | 代理路径没去前缀或鉴权失败 | 确认 `proxy_pass http://127.0.0.1:19999/;` 尾部斜杠（去 /api 前缀的关键） |
| WebSocket 连不上 | 缺升级头 | 确认 `/ws` location 有 `proxy_set_header Upgrade/Connection` |
| 刷新页面 404 | SPA 回退没配 | 确认 `try_files $uri $uri/ /index.html;` |
| 服务反复重启 | 中间件没就绪 | `journalctl -u go-admin` 定位；首次部署迁移与监听并发，有极短建表窗口 |
| 服务器又 OOM | 中间件默认配置 | 重跑第 2 节调优；`free -m` 看 swap 是否生效 |
