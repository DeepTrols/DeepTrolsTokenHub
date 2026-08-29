# ---- Build stage ----
FROM golang:1.22-alpine AS builder

WORKDIR /app

# 国内网络环境可访问的 Go 模块镜像（可用 --build-arg GOPROXY=... 覆盖）
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /app/server ./cmd/api && \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /app/worker ./cmd/worker

# ---- Runtime ----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=builder /app/server .
COPY --from=builder /app/worker .
COPY --from=builder /app/migrations ./migrations

EXPOSE 8080

ENTRYPOINT ["./server"]
