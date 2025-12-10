# Docker 快速启动指南

## 📦 部署模式选择

Ember 提供两种 Docker 部署模式：

| 模式 | 配置文件 | 适用场景 | 数据库 |
|------|---------|---------|--------|
| **生产模式** | `docker-compose.yaml` | 生产环境、远程数据库 | 使用远程 PostgreSQL |
| **本地模式** | `docker-compose.local.yml` | 本地开发、快速体验 | 包含本地 PostgreSQL |

---

## 🚀 模式一：生产部署（推荐）

**使用远程 PostgreSQL 数据库**

```bash
# 1. 执行数据库迁移（本地执行，仅首次或更新时需要）
npm install
npx prisma migrate deploy

# 2. 构建并启动容器
docker compose up -d --build

# 3. 查看日志
docker compose logs -f ember
```

访问：http://localhost:3000

---

## 🏠 模式二：本地开发

**包含本地 PostgreSQL 数据库**

```bash
# 1. 配置 .env 文件（设置本地数据库密码）
echo "POSTGRES_PASSWORD=your-secure-password" >> .env

# 2. 执行数据库迁移
npm install
npx prisma migrate deploy

# 3. 启动所有服务（应用 + 数据库）
docker compose -f docker-compose.local.yml up -d --build

# 4. 查看日志
docker compose -f docker-compose.local.yml logs -f
```

访问：http://localhost:3000

> **注意**：本地模式会在 Docker 中启动 PostgreSQL，数据存储在 `postgres_data` volume 中。

## 📋 配置说明

### 环境变量（.env 文件）

确保以下环境变量已配置：

```bash
# 数据库连接（远程 PostgreSQL）
DATABASE_URL="postgresql://用户名:密码@主机:端口/数据库名"

# 应用配置
JWT_SECRET="your-secret-key-min-32-chars"
NODE_ENV=production

# Emby 服务器
EMBY_URL="http://your-emby-server:8096"
EMBY_API_KEY="your-emby-api-key"

# 应用URL
NEXT_PUBLIC_APP_URL="http://localhost:3000"
NEXT_PUBLIC_EMBY_URL="https://your-emby-public-url"
```

---

## 🔧 常用命令

### 生产模式命令

```bash
# 查看容器状态
docker compose ps

# 查看实时日志
docker compose logs -f ember

# 重启服务
docker compose restart

# 停止服务
docker compose down

# 更新代码后重新部署
git pull
npx prisma migrate deploy  # 如果有新的迁移
docker compose up -d --build
```

### 本地模式命令

```bash
# 查看容器状态
docker compose -f docker-compose.local.yml ps

# 查看所有服务日志
docker compose -f docker-compose.local.yml logs -f

# 只查看应用日志
docker compose -f docker-compose.local.yml logs -f ember

# 只查看数据库日志
docker compose -f docker-compose.local.yml logs -f postgres

# 停止所有服务
docker compose -f docker-compose.local.yml down

# 停止并删除数据卷（⚠️ 会删除数据库数据）
docker compose -f docker-compose.local.yml down -v
```

---

## 🐛 故障排查

### 容器无法启动

```bash
# 查看详细日志
docker compose logs ember

# 检查环境变量
docker compose config

# 重新构建（无缓存）
docker compose build --no-cache
```

### 数据库连接失败

1. 检查 .env 文件中的 DATABASE_URL 格式
2. 确认数据库服务器可访问：`ping 数据库主机`
3. 检查防火墙规则

### 健康检查失败

等待 40 秒启动时间后再检查：

```bash
# 手动测试健康检查
curl http://localhost:3000/api/health
```

## 📊 架构说明

### 镜像特性

- **多阶段构建**：最小化镜像体积（~200MB）
- **非 root 用户**：nextjs:1001，增强安全性
- **Standalone 输出**：Next.js 优化模式
- **健康检查**：自动检测服务可用性

### 为什么不在容器内执行迁移？

❌ **不推荐**：容器启动时自动迁移
- 并发启动时的竞态条件
- 迁移失败阻止应用启动
- 生产镜像包含开发依赖

✅ **推荐**：本地或 CI/CD 中执行迁移
- 迁移和应用启动解耦
- 失败时易于调试
- 镜像保持纯净

## 🔄 更新流程

```bash
# 1. 拉取最新代码
git pull

# 2. 执行数据库迁移（如果有）
npx prisma migrate deploy

# 3. 重新构建并启动
docker compose up -d --build

# 4. 验证
curl http://localhost:3000/api/health
```

## 📝 注意事项

1. **.env 文件不会被打包到镜像中**（安全考虑）
2. **数据库迁移必须在启动前完成**
3. **首次启动需等待 40 秒**（健康检查 start_period）
4. **生产环境建议使用反向代理**（Nginx/Caddy + SSL）
