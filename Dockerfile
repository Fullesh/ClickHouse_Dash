# Multi-stage Docker build for ClickHouse Platform Control Plane
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Кэшируем зависимости
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходники и собираем статический бинарник
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/clickhouse-platform ./main.go

# Финальный легковесный образ на Alpine Linux
FROM alpine:3.19

# Утилиты pkill и fuser для работы инжектора хаоса в Linux
RUN apk add --no-cache procps psmisc iptables ca-certificates curl

WORKDIR /app

COPY --from=builder /app/clickhouse-platform /app/
COPY --from=builder /app/web /app/web

EXPOSE 8080

ENTRYPOINT ["/app/clickhouse-platform"]
