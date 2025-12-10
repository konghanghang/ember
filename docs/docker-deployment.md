# Docker 部署指南

## 快速启动

### 1. 准备环境变量

确保项目根目录存在 `.env` 文件，包含以下必需配置：

```bash
# 数据库连接（远程 PostgreSQL）
DATABASE_URL="postgresql://用户名:密码@主机:端口/数据库名"

# JWT 密钥
JWT_SECRET="your-secret-key"

# Emby 服务器配置
EMBY_SERVER_URL="http://your-emby-server:8096"
EMBY_API_KEY="your-emby-api-key"

# 应用配置
NODE_ENV=production
NEXT_PUBLIC_APP_NAME="Ember"
```

### 2. 执行数据库迁移

**⚠️ 重要**：首次部署前必须手动执行数据库迁移

```bash
# 方式一：在本地执行（推荐）
npm install
npx prisma migrate deploy

# 方式二：使用临时容器执行
docker compose run --rm ember npx prisma migrate deploy
```

### 3. 构建并启动服务

```bash
# 构建镜像并启动容器
docker compose up -d --build

# 查看日志
docker compose logs -f ember

# 检查健康状态
docker compose ps
```

### 4. 验证部署

访问 http://localhost:3000

- 健康检查端点：http://localhost:3000/api/health
- 登录页面：http://localhost:3000/login

## 常用命令

```bash
# 停止服务
docker compose down

# 重启服务
docker compose restart

# 查看实时日志
docker compose logs -f ember

# 进入容器 Shell
docker compose exec ember sh

# 重新构建镜像
docker compose build --no-cache

# 更新代码后重新部署
git pull
docker compose up -d --build
```

## 数据库管理

```bash
# 查看数据库迁移状态
docker compose run --rm ember npx prisma migrate status

# 执行新的迁移
docker compose run --rm ember npx prisma migrate deploy

# 生成 Prisma Client（通常不需要，已在构建时完成）
docker compose run --rm ember npx prisma generate
```

## 故障排查

### 应用无法启动

1. 检查日志：`docker compose logs ember`
2. 验证 .env 文件配置是否正确
3. 确认数据库连接是否可达：`docker compose exec ember npx prisma db push`

### 数据库连接失败

1. 检查 DATABASE_URL 格式是否正确
2. 确认远程数据库防火墙规则
3. 测试网络连通性：`docker compose exec ember ping 数据库主机`

### 健康检查失败

1. 等待应用完全启动（start_period: 40s）
2. 检查 /api/health 端点是否响应：`curl http://localhost:3000/api/health`
3. 查看应用日志是否有错误

## 生产环境建议

1. **环境变量管理**：使用 Docker Secrets 或外部密钥管理服务
2. **数据库迁移**：在部署流水线中独立执行，不要在容器启动时自动运行
3. **反向代理**：使用 Nginx/Caddy 处理 SSL 和负载均衡
4. **监控**：配置日志收集和性能监控
5. **备份**：定期备份数据库

## 架构说明

### 多阶段构建

Dockerfile 使用三阶段构建优化镜像大小：

1. **deps**：安装生产依赖
2. **builder**：安装所有依赖、生成 Prisma Client、构建 Next.js
3. **runner**：仅复制运行时必需文件，最小化镜像体积

### 安全特性

- 使用非 root 用户运行（nextjs:1001）
- .dockerignore 排除敏感文件
- 健康检查确保服务可用性
- standalone 输出模式减少依赖
