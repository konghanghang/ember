# 多阶段构建 Dockerfile for Ember Go API

# ==================== 构建阶段 ====================
FROM golang:1.24.13-alpine AS builder

# 安装构建依赖
RUN apk add --no-cache git make

# 设置工作目录
WORKDIR /build

# 复制 go.mod 和 go.sum
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建应用
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ember cmd/server/main.go

# ==================== 运行阶段 ====================
FROM alpine:latest

# 安装运行时依赖
RUN apk --no-cache add ca-certificates tzdata wget

# 设置时区
ENV TZ=Asia/Shanghai

# 创建非 root 用户
RUN addgroup -g 1000 ember && \
    adduser -D -u 1000 -G ember ember

# 设置工作目录
WORKDIR /app

# 从构建阶段复制二进制文件
COPY --from=builder /build/ember .

# 创建日志目录
RUN mkdir -p /app/logs && chown -R ember:ember /app

# 切换到非 root 用户
USER ember

# 暴露端口
EXPOSE 8080

# 健康检查
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q -O /dev/null http://localhost:8080/health || exit 1

# 启动应用
CMD ["./ember"]
