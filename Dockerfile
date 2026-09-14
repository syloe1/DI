FROM golang:1.25.8-alpine AS builder

WORKDIR /src

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
