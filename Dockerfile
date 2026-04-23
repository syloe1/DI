FROM golang:1.25.8 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server ./cmd/server

FROM debian:stable-slim

WORKDIR /app

COPY --from=builder /app/server /app/server
COPY --from=builder /app/config /app/config

EXPOSE 19999

CMD ["/app/server"]
