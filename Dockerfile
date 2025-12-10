# ==========================================
# Stage 1: 构建
# ==========================================
FROM node:20-alpine AS builder

WORKDIR /app

# 复制依赖定义文件
COPY package.json package-lock.json* ./

# 安装所有依赖（包括 devDependencies）
RUN npm ci

# 复制源代码
COPY . .

# 生成 Prisma Client（使用虚拟 DATABASE_URL，仅用于构建）
ENV DATABASE_URL="postgresql://placeholder:placeholder@localhost:5432/placeholder"
RUN npx prisma generate

# 构建 Next.js 应用
RUN npm run build

# ==========================================
# Stage 2: 运行时
# ==========================================
FROM node:20-alpine AS runner

WORKDIR /app

# 设置环境变量
ENV NODE_ENV=production \
    PORT=3000 \
    HOSTNAME="0.0.0.0"

# 创建非 root 用户
RUN addgroup --system --gid 1001 nodejs && \
    adduser --system --uid 1001 nextjs

# 复制 Next.js standalone 输出（包含精简的运行时依赖）
COPY --from=builder /app/.next/standalone ./
COPY --from=builder /app/.next/static ./.next/static

# 注意：public 目录为空，且 standalone 模式会自动处理静态文件
# 如果将来需要添加静态资源，取消下面这行的注释：
# COPY --from=builder /app/public ./public

# 复制 Prisma 文件（用于运行迁移）
COPY --from=builder /app/prisma ./prisma

# 复制 Prisma Client（已生成，standalone 可能不包含）
COPY --from=builder /app/node_modules/.prisma ./node_modules/.prisma
COPY --from=builder /app/node_modules/@prisma ./node_modules/@prisma

# 切换到非 root 用户
USER nextjs

# 暴露端口
EXPOSE 3000

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=40s --retries=3 \
  CMD node -e "require('http').get('http://localhost:3000/api/health', (r) => {process.exit(r.statusCode === 200 ? 0 : 1)})"

# 启动应用
CMD ["node", "server.js"]
