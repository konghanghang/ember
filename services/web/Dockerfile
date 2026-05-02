# ==================== 第一阶段：构建 ====================
FROM node:20.18.1-alpine AS builder

WORKDIR /app

# 复制依赖文件
COPY package*.json ./

# 安装依赖
RUN npm ci || npm install

# 复制源代码
COPY . ./

# 构建 Vue 应用（生成静态文件）
ENV NODE_ENV=production
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
