# Benchmarks

This directory documents how to run local capacity tests for the current single-instance service.

## Prerequisites

Start dependencies configured in `config/config.yaml`:

- MySQL: `127.0.0.1:3307`
- Redis: `127.0.0.1:16379`
- RabbitMQ: `127.0.0.1:5672`

Then start the app:

```bash
go run ./cmd/server
```

The HTTP server should listen on:

```text
http://127.0.0.1:19999
```

## Smoke Test

```bash
go run ./cmd/bench -mode=http -url=http://127.0.0.1:19999/post/list -c=10 -n=100
```

## HTTP Read Baseline

```bash
go run ./cmd/bench -mode=http -url=http://127.0.0.1:19999/post/list -c=50 -n=5000
go run ./cmd/bench -mode=http -url=http://127.0.0.1:19999/post/list -c=100 -n=10000
go run ./cmd/bench -mode=http -url=http://127.0.0.1:19999/post/list -c=200 -n=20000
```

Record:

- RPS
- p50
- p95
- p99
- error count
- MySQL CPU / slow SQL
- app CPU / memory

## Authenticated Group Message Baseline

Prepare:

- one valid JWT
- one normal group
- current user is an active member

Run:

```bash
go run ./cmd/bench \
  -mode=http \
  -url=http://127.0.0.1:19999/auth/groups/1/messages \
  -method=POST \
  -token="<JWT>" \
  -body="{\"content\":\"benchmark group message\"}" \
  -c=20 \
  -n=1000
```

Increase concurrency gradually:

```bash
go run ./cmd/bench -mode=http -url=http://127.0.0.1:19999/auth/groups/1/messages -method=POST -token="<JWT>" -body="{\"content\":\"benchmark group message\"}" -c=50 -n=5000
go run ./cmd/bench -mode=http -url=http://127.0.0.1:19999/auth/groups/1/messages -method=POST -token="<JWT>" -body="{\"content\":\"benchmark group message\"}" -c=100 -n=10000
```

Watch metrics:

```text
http://127.0.0.1:6060/metrics
```

Key metrics:

- `chat_outbox_pending`
- `chat_outbox_processing`
- `chat_outbox_failed`
- `chat_consumer_failed_total`
- `chat_push_dropped_slow_total`

## WebSocket Ping Baseline

Prepare one valid JWT.

```bash
go run ./cmd/bench \
  -mode=ws \
  -url=ws://127.0.0.1:19999/ws?token=<JWT> \
  -ws-message="{\"type\":\"ping\"}" \
  -c=50 \
  -n=1000
```

## Capacity Rule

Use p99 latency and error rate as the stopping condition:

```text
single_instance_safe_qps = max_stable_qps * 0.5
```

Example:

```text
If a single instance can sustain 2000 req/s with p99 < 200ms and error rate < 0.1%,
use 1000 req/s as the safe production planning value.
```

## Baseline Record

| Date | Scenario | Concurrency | Total | RPS | p50 | p95 | p99 | Errors | Notes |
| --- | --- | ---: | ---: | ---: | --- | --- | --- | ---: | --- |
| TBD | HTTP post list | 50 | 5000 | TBD | TBD | TBD | TBD | TBD | TBD |
| TBD | Group message send | 20 | 1000 | TBD | TBD | TBD | TBD | TBD | TBD |
| TBD | WebSocket ping | 50 | 1000 | TBD | TBD | TBD | TBD | TBD | TBD |
