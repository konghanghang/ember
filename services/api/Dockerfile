# 多阶段构建 Dockerfile for Ember Go API
#
# 注意：本 Dockerfile 的 build context 是仓库根（不是 services/api/）。
# 原因是 runtime 阶段需要 COPY 仓库根的 infrastructure/database/*.sql 进镜像，
# 这些 SQL 是 schema 真相源，必须随镜像一起分发以便启动期 Migrate 自动应用。
#
# 调用方式：
#   docker build -f services/api/Dockerfile -t ember-api .
#
# 仓库根 .dockerignore 会限制实际进入构建上下文的内容范围。

# ==================== 构建阶段 ====================
FROM golang:1.24.13-alpine AS builder

# 安装构建依赖
RUN apk add --no-cache git make

# 设置工作目录
WORKDIR /build

# 先单独拷贝 go.mod / go.sum 以最大化 Docker layer 缓存命中
COPY services/api/go.mod services/api/go.sum ./

# 下载依赖
RUN go mod download

# 复制后端源码
COPY services/api/. .

# 构建统一生产入口；API 与 Gateway 通过运行时子命令选择进程角色。
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ember ./cmd/ember

# ==================== 运行阶段 ====================
FROM alpine:3.22

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

# 拷贝仓库根 infrastructure/database/ 顶层 SQL 至镜像内 /app/migrations。
# 通配符 *.sql 仅匹配顶层文件，archive/ 子目录不会被拷入。
COPY infrastructure/database/*.sql /app/migrations/

# 启动期 Migrate 阶段读取此目录；与镜像 tag 强绑定（镜像内 SQL 是 schema 真相）。
ENV EMBER_MIGRATIONS_DIR=/app/migrations

# 创建日志目录并交还所属
RUN mkdir -p /app/logs && chown -R ember:ember /app

# 切换到非 root 用户
USER ember

# 暴露 API 与 Gateway 容器内固定端口；实际服务每次只监听其中一个。
EXPOSE 8080 8081

# 健康检查
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q -O /dev/null http://localhost:8080/health || exit 1

# 单镜像、单二进制：默认 API；Compose Gateway 只覆盖 CMD 为 gateway。
ENTRYPOINT ["./ember"]
CMD ["api"]
