# Monitoring

This directory contains a minimal Prometheus and Grafana setup for chat reliability metrics.

## Metrics Endpoint

The application exposes metrics from the pprof server:

```text
http://127.0.0.1:6060/metrics
```

## Prometheus

### Docker Compose

From this directory:

```bash
docker compose -f docker-compose.monitoring.yml up -d
```

Open:

```text
http://localhost:9090
```

Prometheus scrapes the app from Docker through:

```text
host.docker.internal:6060
```

Make sure the Go service exposes:

```text
http://127.0.0.1:6060/metrics
```

### Local Binary

Use:

```bash
prometheus --config.file=deploy/monitoring/prometheus.yml
```

When Prometheus runs in Docker on Windows/macOS, `host.docker.internal:6060` points back to the host machine.

If Prometheus runs directly on the same machine as the app, change the target in `prometheus.yml` to:

```yaml
targets:
  - 127.0.0.1:6060
```

## Alerts

Alert rules are in:

```text
deploy/monitoring/alerts.yml
```

They cover:

- outbox backlog
- outbox publish failures
- consumer failures
- slow websocket client drops
- online connection abnormal drop
- failed outbox rows

## Grafana

Docker Compose starts Grafana at:

```text
http://localhost:3000
```

Default login:

```text
admin / admin
```

The Prometheus data source and dashboard are provisioned automatically.

If you run Grafana manually, import:

```text
deploy/monitoring/grafana-dashboard.json
```

The dashboard expects a Prometheus data source.

## Stop

```bash
docker compose -f docker-compose.monitoring.yml down
```
