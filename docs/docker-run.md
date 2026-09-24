# Docker Compose 启动说明

> **定位：本地快速联调专用，不是生产部署方式。**
> 生产环境走裸机路线（编译好的二进制 + systemd，复用服务器上已有的
> MySQL / Redis / RabbitMQ），见 [deploy-native.md](deploy-native.md)。

本项目提供根目录 `docker-compose.yaml`，用于一键启动：

- Go API 服务
- MySQL
- Redis
- RabbitMQ
- Swagger UI

## 启动

在项目根目录执行：

```bash
docker compose up -d --build
```

## 访问地址

| 服务 | 地址 |
| --- | --- |
| API | http://localhost:19999 |
| Swagger UI | http://localhost:8288 |
| Metrics | http://localhost:6060/metrics |
| pprof | http://localhost:6060/debug/pprof |
| RabbitMQ Management | http://localhost:15672 |

RabbitMQ 默认账号：

```text
guest / guest
```

## Swagger 文档

Swagger UI 会自动加载：

```text
docs/openapi.yaml
```

如果修改了接口文档，重启 Swagger UI：

```bash
docker compose restart swagger-ui
```

## Windows 端口被系统预留

Windows 上 Hyper-V / WSL 会动态预留一批 TCP 端口段，落在其中的端口 Docker 无法绑定，
报错形如：

```text
ports are not available: exposing port TCP 0.0.0.0:8081 -> 127.0.0.1:0:
listen tcp 0.0.0.0:8081: bind: An attempt was made to access a socket in a way
forbidden by its access permissions.
```

查看当前被排除的端口段（需要管理员权限运行 `netsh`）：

```bash
netsh interface ipv4 show excludedportrange protocol=tcp
```

Swagger UI 的宿主端口因此默认使用 `8288`（8081 常落在 `8081-8180` 预留段内）。
如需改端口，编辑 `docker-compose.yaml` 中 `swagger-ui.ports` 的左侧值，
选一个不在排除列表里的端口，然后：

```bash
docker compose up -d swagger-ui
```

## 停止

```bash
docker compose down
```

如果要清空 MySQL、Redis、RabbitMQ 数据：

```bash
docker compose down -v
```

## 敏感数据

`docker-compose.yaml` 中的 MySQL 密码 / JWT 密钥使用环境变量引用
（`${MYSQL_PASSWORD:-123456}`）：复制 `.env.example` 为 `.env` 并填入真实值即可
（`.env` 已被 `.gitignore` 忽略）。不要直接改 yaml 里的默认值提交 Git。

## 小内存服务器（2C2G）

2C2G 上跑全家桶需要调优（RabbitMQ 限内存 + MySQL 瘦身 + swap 兜底），
详见 [deploy-2c2g.md](deploy-2c2g.md)。
