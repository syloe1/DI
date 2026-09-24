# 裸机部署（不用 Docker）

> **适用场景**：生产环境。服务器直接跑编译好的二进制，由 systemd 托管；
> MySQL / Redis / RabbitMQ 复用服务器上已有的实例，不额外起容器。
>
> **与另外两份文档的关系**：
> - [docker-run.md](docker-run.md) —— 本地快速联调专用（一键起全家桶），**不代表生产**。
> - [deploy-2c2g.md](deploy-2c2g.md) —— 2C2G 内存调优（RabbitMQ/MySQL/swap），
>   本文的部署方式与之配合使用。

## 1. 路线与前提

```text
本地 Windows                       服务器（2C2G，已有中间件）
─────────────                     ──────────────────────────
go build (linux/amd64)  ──scp──▶  /opt/go-admin/go-admin
                                  systemd: go-admin.service
                                        ├─▶ MySQL      (已有实例，复用)
                                        ├─▶ Redis      (已有实例，复用)
                                        └─▶ RabbitMQ   (已有实例，复用)
```

前提（服务器上已具备，本文不涉及安装）：

- 已有可用实例的 MySQL、Redis、RabbitMQ（端口分别是 3306 / 6379 / 5672）
- 有 root 或 sudo 权限
- **服务器上不需要装 Go**：二进制在本地交叉编译（`CGO_ENABLED=0`，纯静态）

代码本身已具备的能力（无需额外配置）：SQL 迁移编译进二进制启动自动执行
（[migrate.go:31](../pkg/core/migrate.go#L31) 用 `embed` 读 SQL）、日志只写 stdout
交给 journald 收集、收到 SIGTERM 后 10s 内优雅排空
（[main.go:98](../cmd/server/main.go#L98)）。

## 2. 服务器一次性准备

以下命令在服务器上以 root 执行。

### ① 建专用系统用户与目录

```bash
useradd --system --no-create-home --shell /usr/sbin/nologin go-admin
mkdir -p /opt/go-admin/config
```

目录与二进制都归 root，服务用户 `go-admin` 只有读+执行权限，最小权限。

### ② 建库（迁移不会替你建库）

`pkg/core/migrate.go` 是对**已经连上的库**执行 `m.Up()`，DSN 里 `dbname` 不存在时
连接阶段就会失败。所以先建库：

```sql
CREATE DATABASE IF NOT EXISTS `go`
  DEFAULT CHARACTER SET utf8mb4
  COLLATE utf8mb4_general_ci;
```

> 更稳的做法是建专用账号而不是用 root，并只授权给 `go` 库：
> `CREATE USER 'go'@'127.0.0.1' IDENTIFIED BY '<密码>';`
> `GRANT ALL ON go.* TO 'go'@'127.0.0.1';`
> 用非 root 账号时记得在 `go-admin.env` 里覆盖 `APP_MYSQL_USERNAME`。

### ③④⑤ 以下命令在仓库检出目录下执行

即服务器上 `git clone` 出来的那份代码（本文以 `/opt/src/DI` 为例）：

```bash
cd /opt/src/DI
```

### ③ 放配置文件

```bash
cp config/config.example.yaml /opt/go-admin/config/config.yaml
vi /opt/go-admin/config/config.yaml
```

⚠ **必须从 `config.example.yaml` 复制**，不要去拷开发机的 `config/config.yaml`：

- `config/config.yaml` 在 `.gitignore` 里，服务器 `git clone` 后本来就没有它；
  它是开发机上随手可改、不受版本管理的文件，**键可能缺失或过期**。
- 键一旦缺失，对应的 `APP_*` 环境变量会**静默失效**且不报错（原因见第 5 节）。
  `config.example.yaml` 是唯一保证「所有可覆盖键都在」的模板。

这个文件里**只放非敏感项**（host / port / dbname / cors_origins 等），
密码密钥一律留空、走环境变量。

### ④ 放环境变量文件

```bash
cp deploy/go-admin.env.example /opt/go-admin/go-admin.env
chmod 600 /opt/go-admin/go-admin.env
vi /opt/go-admin/go-admin.env      # 至少填 APP_MYSQL_PASSWORD 和 APP_JWT_SECRET
```

注意这是 systemd 的 `EnvironmentFile`，**不是 shell 脚本**：不支持 `export`、
不支持 `$(...)`、不支持 `${VAR}` 展开，值也不要加引号。

### ⑤ 装 systemd 单元

```bash
cp deploy/systemd/go-admin.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable go-admin          # 开机自启（先不 start，等二进制上传）
```

### ⑥ 放行端口

- **19999**：应用 HTTP 端口，需在**云厂商安全组**和服务器本机防火墙放行。
- **6060**：pprof / metrics，[main.go:84](../cmd/server/main.go#L84) 硬编码监听
  `127.0.0.1`，只对本机可见，**不要放行**。要远程看就 SSH 端口转发：
  `ssh -L 6060:127.0.0.1:6060 root@<server>`。

## 3. 日常发布

### 方式 A：本地交叉编译后上传（推荐）

```bash
./deploy/build-native.sh root@<server-ip>
```

脚本做三件事：交叉编译 → `scp` 到 `go-admin.new` → 远程 `mv` + `systemctl restart`。

> 为什么先传 `.new` 再 `mv`：Linux 不允许覆盖正在运行的二进制（`ETXTBSY`），
> 直接 `scp` 到 `go-admin` 会失败；`mv` 走的是 `rename(2)`，旧的运行中进程不受影响。

只想编译不上传：`./deploy/build-native.sh`，产物在 `dist/go-admin`（已在 `.gitignore` 中）。

### 方式 B：服务器上 git pull 后编译

```bash
cd /opt/src/DI && git pull
CGO_ENABLED=0 go build -trimpath -o /opt/go-admin/go-admin ./cmd/server
systemctl restart go-admin
```

代价：服务器要装 Go 1.25 工具链，且 2C2G 上编译会瞬时吃内存——
**必须先按 [deploy-2c2g.md](deploy-2c2g.md) 第 3 节加好 swap**，否则可能被 OOM Killer 干掉。

## 4. 部署后验证

```bash
systemctl status go-admin --no-pager
journalctl -u go-admin -n 50 --no-pager
```

日志里按顺序应出现：`start database migration` → `DI container initialized` →
`server started, port: 19999` → `database migration completed`。

探活：

```bash
curl -sS http://127.0.0.1:19999/healthz   # 进程活着 → {"status":"ok"}
curl -sS http://127.0.0.1:19999/readyz    # MySQL+Redis 都通才返回成功
```

`/readyz` 会实际执行 `SELECT 1` 和 Redis `PING`，是比 `/healthz` 更有意义的
部署验收项——它挂了会返回 503 并说明是 mysql 还是 redis 没就绪。

## 5. 环境变量为什么不生效（viper 的坑）

**结论：`config.yaml` 里没有声明的键，环境变量设了也没用，而且不报错。**

`config.Load()` 用的是 viper 的 `AutomaticEnv()` + `Unmarshal()`
（[config.go:152-163](../config/config.go#L152-L163)）。`Unmarshal` 只会遍历
**配置文件里已存在的键**去查环境变量；配置文件里没有的键，`APP_XXX` 形同不存在。

本仓库实测（临时用例，已删除）：

```text
config.yaml 中有 mysql.password, jwt.secret, 没有 logger 段
  APP_MYSQL_PASSWORD=secret-from-env  → mysql.password = "secret-from-env"  ✓
  APP_JWT_SECRET=jwt-from-env         → jwt.secret     = "jwt-from-env"     ✓
  APP_LOGGER_FORMAT=json              → logger.format  = "console"          ✗ 静默失效
  APP_LOGGER_LEVEL=debug              → logger.level   = "info"             ✗ 静默失效
```

**实际影响**：开发机的 `config/config.yaml` 原先恰好没有 `logger:` 段，导致
`APP_LOGGER_FORMAT=json` 无论如何都不生效、日志一直是带颜色的 console 格式
（该文件已补上 `logger:` 段，实测 `APP_LOGGER_FORMAT=json` 现在能正确覆盖）。
但这只是**治标**——那个文件不受版本管理，任何一次手改都可能再删掉一个键。

**本路线的规避方式**（不改代码）：服务器上从 `config.example.yaml` 复制配置，
**不要删掉任何键**，哪怕值是空串。想新增一个可通过环境变量覆盖的配置项时，
先在 `config.example.yaml` 里加上对应键，再在 `go-admin.env` 里赋值。

> 如果你更想让环境变量「设了就一定生效」，那要改 `config.go`：给每个键显式
> `v.BindEnv("logger.format", "APP_LOGGER_FORMAT")`，或在 `Load` 里
> `v.SetDefault()` 出全部键。这是代码改动，本次按你的选择没做。

## 6. 排错

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| `load config failed: open config/config.yaml: no such file` | systemd 单元里 `WorkingDirectory` 不是应用根目录 | 确认 `WorkingDirectory=/opt/go-admin` 且 `config/config.yaml` 在位 |
| `config: validate: mysql.password is required` | 环境变量注入失败 | 检查 `go-admin.env` 语法（有无 `export`/引号）；确认 `config.yaml` 里**有** `mysql.password` 键 |
| `init mysql failed` | 库不存在 / 账号密码错 / 未授权 | 按 ② 建库；确认 `APP_MYSQL_USERNAME` 与实际账号一致 |
| 服务反复重启 | 中间件没起来或连不上 | 单元里 `Restart=always` 会一直重试，看 `journalctl` 定位是哪个中间件；确认 3306/6379/5672 可达 |
| 设了 `APP_LOGGER_FORMAT=json` 但日志仍是彩色 console | 见第 5 节 viper 陷阱 | 确认 `config.yaml` 有 `logger:` 段 |
| 接口返回时间差 8 小时 | `TZ` 未设置，DSN 用 `loc=Local` 按 UTC 解析 | 设 `TZ=Asia/Shanghai`；本项目未引入 `time/tzdata`，依赖系统 `/usr/share/zoneinfo`（`ls /usr/share/zoneinfo/Asia/Shanghai` 确认存在） |
| 启动正常但报表不存在 | 迁移失败后**进程不会退出**（[main.go:53](../cmd/server/main.go#L53) 是 goroutine，失败只打日志） | 搜日志 `database migration failed`；另外迁移与监听是并发的，首次部署有极短的「表还没建好」窗口 |

## 7. 与 2C2G 调优的关系

裸机部署下，[deploy-2c2g.md](deploy-2c2g.md) 里**仍然有效**的是：

- ① RabbitMQ 限内存 + 关 management 插件
- ② MySQL 瘦身（`innodb_buffer_pool_size = 64M` 等）
- ③ 加 swap 兜底
- ⑤ 突发性能实例的 CPU 积分注意事项

**不再适用**的是该文档第 3 节的容器 `mem_limit` —— 那是给 Docker 路径的。
裸机下对应的内存控制手段是 systemd 单元里的：

```ini
Environment=GOMEMLIMIT=256MiB
```

这是 Go 运行时的软内存上限（超过会让 GC 更激进，不是硬杀）。2C2G 上够用；
想更硬的手段可以再加 `MemoryMax=512M`（硬上限，超了会被 cgroup OOM Killer 杀，
不建议一上来就设）。
