# 2C2G 小内存服务器部署指南

> 适用场景：阿里云 2核2G（轻量应用服务器 / 突发性能实例）。
> 生产以**裸机方式**部署（Go 二进制 + systemd），**MySQL / Redis / RabbitMQ
> 复用服务器上已有实例**，不用 Docker —— 部署步骤见 [deploy-native.md](deploy-native.md)。
> 本文只讲这台小机的内存调优：**调优后中间件可稳定运行**，默认配置会 OOM，
> 必须按下文三步调优。

> 说明：下文的「内存账本」把四个组件都算进去，是因为它们**共用这台 2C2G 机器**；
> 复用已有实例时，你需要确认的是这些实例的现有占用之和是否已接近上限。

## 1. 内存账本

| 组件 | 默认配置（危险） | 调优后（稳定） |
| --- | --- | --- |
| 系统 + SSH | ~250MB | ~250MB |
| MySQL 8（buffer pool 128M） | ~450MB | ~200MB |
| Redis | ~50MB | ~50MB |
| RabbitMQ（watermark 0.4 + management 插件） | ~700MB+ | ~350MB |
| Go 应用 | ~150MB | ~150MB |
| **合计** | **~1.9GB+ → OOM** | **~1.0-1.15GB → 留 ~700MB 余量** |

## 2. 三步调优（不动代码）

### ① RabbitMQ 限内存（最大的内存黑洞）

```ini
# /etc/rabbitmq/rabbitmq.conf
vm_memory_high_watermark.relative = 0.35
```

关闭用不上的管理插件（管理 Web 面板本身占内存）：

```bash
rabbitmq-plugins disable rabbitmq_management
```

### ② MySQL 瘦身

```ini
# /etc/mysql/conf.d/2g.cnf
[mysqld]
innodb_buffer_pool_size = 64M
performance_schema = OFF
max_connections = 50
```

### ③ 加 swap 兜底（防瞬时峰值触发 OOM）

```bash
fallocate -l 2G /swapfile
chmod 600 /swapfile
mkswap /swapfile
swapon /swapfile
echo '/swapfile none swap sw 0 0' >> /etc/fstab
```

## 3. 生产部署方式：裸机，不用 Docker

本项目的生产路线是**编译好的二进制 + systemd 托管，复用服务器上已有的
MySQL / Redis / RabbitMQ**，不在小机上跑容器。完整步骤见
[deploy-native.md](deploy-native.md)。

裸机下对应这一节的内存控制点是：

- **中间件内存**：靠上文 ① RabbitMQ 限内存、② MySQL 瘦身
- **应用自身内存**：systemd 单元里的 `Environment=GOMEMLIMIT=256MiB`
  （Go 运行时的软上限，超过会让 GC 更激进，不是硬杀；想要硬的再加
  `MemoryMax=512M`，但被 cgroup OOM Killer 杀掉会导致重启，不建议一上来就设）

> 根目录 `docker-compose.yaml` 只用于**本地快速联调**。如果确实想在 2C2G 上
> 用 Docker 起全家桶，需要给容器加 `mem_limit` 并把 RabbitMQ 换成
> `rabbitmq:3.13-alpine`（省 ~200MB）——但容器自身还有额外开销，小机上不划算。

## 4. 敏感数据：环境变量注入

两种部署方式都遵循同一原则：**密码和密钥只走环境变量，不写进入 git 的配置**。

**本地 Docker Compose**：`docker-compose.yaml` 中的密码已改为环境变量引用
（`${MYSQL_PASSWORD:-123456}`）：

1. 复制模板：`cp .env.example .env`
2. 修改 `.env` 中的真实密码/密钥（`.env` 已被 `.gitignore` 忽略，不会提交）

**生产裸机**：配置在 `/opt/go-admin/go-admin.env`，由 systemd 的
`EnvironmentFile` 注入：

1. 复制模板：`cp deploy/go-admin.env.example /opt/go-admin/go-admin.env`
2. 填入真实密码/密钥，`chmod 600`
3. 业务配置 `config/config.yaml`（在服务器上从 `config.example.yaml` 复制，
   本身不入 git）只放非敏感项

> ⚠ 有个 viper 的坑会让环境变量**静默失效**：`config.yaml` 里没声明的键，
> `APP_*` 环境变量设了也不生效且不报错。所以那个文件不能删键——详见
> [deploy-native.md 第 5 节](deploy-native.md)。

## 5. 阿里云突发性能实例注意事项

- 2C2G 轻量应用服务器多为**突发性能实例（t5/t6 系列）**，CPU 积分制：
  日常低负载没问题，长时间满载会烧积分导致 CPU 被限速。
- 本项目的日常流量（个人 IM 演示/面试展示）远达不到限速阈值；
  **不要在小机上跑压测（bench）或全量备份高峰作业**。
- 建议开启监控告警（内存 > 80%、CPU > 90%）兜底。

## 6. 定位：2C2G vs J1900 8G

| 方案 | 定位 | 适合 |
| --- | --- | --- |
| 阿里云 2C2G | 线上演示入口（面试官可访问） | 展示项目、日常开发联调 |
| J1900 8G 小主机（农村老家 + cloudflared 内网穿透） | 完整环境常驻 | 长期跑全量数据、多实例演练 |

两者可以共存：2C2G 放公网入口，J1900 做本地完整环境；或二选一（预算紧张
时 2C2G 调优后完全够个人项目用）。
