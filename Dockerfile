FROM golang:1.25.8-alpine AS builder

WORKDIR /src

# 默认 proxy.golang.org 在部分网络下不可达，改用国内公共代理；
# 构建机可访问官方代理时用 --build-arg GOPROXY=https://proxy.golang.org,direct 覆盖。
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/go-admin ./cmd/server

FROM alpine:3.21

WORKDIR /app

COPY --from=builder /out/go-admin /app/go-admin
COPY config/config.yaml /app/config/config.yaml

EXPOSE 19999 6060

CMD ["/app/go-admin"]
