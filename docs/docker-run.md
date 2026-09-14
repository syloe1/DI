# Docker Compose 启动说明

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
| Swagger UI | http://localhost:8081 |
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

## 停止

```bash
docker compose down
```

如果要清空 MySQL、Redis、RabbitMQ 数据：

```bash
docker compose down -v
```
