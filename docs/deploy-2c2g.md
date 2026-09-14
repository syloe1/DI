# 2C2G 小内存服务器部署指南

> 适用场景：阿里云 2核2G（轻量应用服务器 / 突发性能实例）跑本项目全家桶
> （Go API + MySQL + Redis + RabbitMQ）。结论先行：**调优后可稳定运行**，
> 默认配置会 OOM，必须按下文三步调优。

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

## 3. Docker Compose 小内存模式

根目录 `docker-compose.yaml` 是开发默认（RabbitMQ 带 management 插件）。
2C2G 上建议改用精简镜像并限制内存：

```yaml
rabbitmq:
  image: rabbitmq:3.13-alpine          # 无 management 插件，省 ~200MB
  mem_limit: 512m

mysql:
  mem_limit: 512m

redis:
  mem_limit: 256m
```

> 提示：`mem_limit` 是硬上限，容器超限会被 OOM；建议先用 `docker stats` 观察
> 实际占用再收紧。swap 仍建议保留，作为瞬时峰值保险。

## 4. 敏感数据：.env 用法

`docker-compose.yaml` 中的密码已改为环境变量引用（`${MYSQL_PASSWORD:-123456}`）：

1. 复制模板：`cp .env.example .env`
2. 修改 `.env` 中的真实密码/密钥（`.env` 已被 `.gitignore` 忽略，不会提交）
3. 业务配置（`config/config.yaml`）同理：复制 `config.example.yaml`，用
   `APP_MYSQL_PASSWORD` / `APP_JWT_SECRET` 环境变量注入，文件本身不入库

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
