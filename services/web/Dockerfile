# ==================== 第一阶段：构建 ====================
# Vue 产物只有静态文件；在构建机原生架构运行 Node，避免 arm64 构建经过 QEMU。
FROM --platform=$BUILDPLATFORM node:22.12.0-alpine AS builder

WORKDIR /app

# 复制依赖文件
COPY package*.json ./

# 严格使用锁文件；安装失败立即终止，不回退到可能重新解析依赖的 npm install。
RUN npm ci

# 复制源代码
COPY . ./

# 构建 Vue 应用（生成静态文件）
ARG VITE_GIT_COMMIT_SHA=""
ARG VITE_GITHUB_REPOSITORY="konghanghang/ember"
ARG VITE_GITHUB_REPOSITORY_URL=""
ENV NODE_ENV=production
ENV VITE_GIT_COMMIT_SHA=${VITE_GIT_COMMIT_SHA}
ENV VITE_GITHUB_REPOSITORY=${VITE_GITHUB_REPOSITORY}
ENV VITE_GITHUB_REPOSITORY_URL=${VITE_GITHUB_REPOSITORY_URL}
RUN npm run build

# ==================== 第二阶段：运行 ====================
FROM nginx:1.27.5-alpine

# 安装运行时依赖
RUN apk --no-cache add ca-certificates tzdata curl

# 从构建阶段复制构建产物
COPY --from=builder /app/dist /usr/share/nginx/html

# 复制 nginx 配置
COPY nginx.conf /etc/nginx/conf.d/default.conf

# 设置环境变量
ENV TZ=Asia/Shanghai

# 暴露端口
EXPOSE 80

# 健康检查
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD curl -f http://localhost/ || exit 1

# Nginx 会自动启动
CMD ["nginx", "-g", "daemon off;"]
