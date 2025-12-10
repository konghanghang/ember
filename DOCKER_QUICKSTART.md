# Docker 快速启动指南

## 📦 部署模式选择

Ember 提供三种 Docker 部署模式：

| 模式 | 配置文件 | 适用场景 | 镜像来源 |
|------|---------|---------|---------|
| **生产模式** | `docker-compose.yaml` | 生产环境 | 本地构建 |
| **本地模式** | `docker-compose.local.yml` | 本地开发 | 本地构建 |
| **预构建镜像** | 修改 compose 文件 | 快速部署/测试 | GitHub Packages |

---

## 🎯 模式零：使用预构建镜像（最快）

**直接使用 GitHub 自动构建的镜像，无需本地构建**

### 测试环境（最新开发版）

```bash
# 1. 修改 docker-compose.yaml，将 build 替换为 image
services:
  ember:
    image: ghcr.io/konghanghang/ember:master  # 使用最新 master 分支镜像
    # build:
    #   context: .
    #   dockerfile: Dockerfile
    container_name: ember-app
    restart: unless-stopped
    ports:
      - "3000:3000"
    env_file:
      - .env

# 2. 拉取并启动
docker compose pull
docker compose up -d

# 3. 查看日志
docker compose logs -f ember
```

### 生产环境（稳定版本）

```bash
# 使用特定版本的生产镜像
services:
  ember:
    image: ghcr.io/konghanghang/ember:v1.0.0  # 或 :latest
    container_name: ember-app
    restart: unless-stopped
    ports:
      - "3000:3000"
    env_file:
      - .env

# 拉取并启动
docker compose pull
docker compose up -d
```

**优点**：
- ✅ 无需本地构建（节省 3-5 分钟）
- ✅ 镜像经过 CI 验证
- ✅ 适合快速部署和测试

**镜像标签说明**：
- `:master` - 最新开发版本（测试用，每次 push master 自动更新）
- `:latest` - 最新稳定版本（生产推荐）
- `:v1.0.0` - 特定版本（生产推荐）

---

## 🚀 模式一：生产部署（推荐）

**使用远程 PostgreSQL 数据库**

```bash
# 1. 执行数据库迁移（首次部署或更新时需要）
# SQL 文件位置：prisma/migrations/*/migration.sql
psql $DATABASE_URL -f prisma/migrations/20251207010855_ember/migration.sql

# 2. 构建并启动应用
docker compose up -d --build

# 3. 查看日志
docker compose logs -f ember
```

访问：http://localhost:3000

> **💡 关于数据库迁移**：
> - Prisma 生成的标准 SQL 文件在 `prisma/migrations/*/migration.sql`
> - 直接用 psql 或你熟悉的工具执行
> - 应用镜像：351MB（纯运行时，不含任何迁移工具）

---

## 🏠 模式二：本地开发

**包含本地 PostgreSQL 数据库**

```bash
# 1. 配置 .env 文件（设置本地数据库密码）
echo "POSTGRES_PASSWORD=your-secure-password" >> .env

# 2. 启动所有服务（应用 + 数据库）
docker compose -f docker-compose.local.yml up -d --build

# 3. 执行数据库迁移
psql $DATABASE_URL -f prisma/migrations/20251207010855_ember/migration.sql

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
psql $DATABASE_URL -f prisma/migrations/新迁移目录/migration.sql  # 如果有新的迁移
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

- **应用镜像**：351MB（纯运行时）
  - 多阶段构建，Next.js standalone 输出
  - 只包含 Prisma Client，无任何迁移工具
  - 非 root 用户（nextjs:1001）
  - 内置健康检查

### 数据库迁移

**Prisma 生成的 SQL 文件**：
- 位置：`prisma/migrations/*/migration.sql`
- 标准 PostgreSQL SQL，可用任何工具执行
- 无需额外容器或工具

**执行方式**：
```bash
# 使用 psql（推荐）
psql $DATABASE_URL -f prisma/migrations/20251207010855_ember/migration.sql

# 或使用你熟悉的数据库工具（DBeaver、pgAdmin 等）
```

**优势**：
- ✅ **极简主义**：应用镜像 351MB，无冗余工具
- ✅ **灵活性**：用你熟悉的方式执行 SQL
- ✅ **透明性**：直接看到执行的 SQL，无黑盒
- ✅ **无依赖**：不需要 Node.js、Prisma CLI 或额外容器

## 🔄 更新流程

```bash
# 1. 拉取最新代码
git pull

# 2. 执行数据库迁移（如果有新的迁移）
psql $DATABASE_URL -f prisma/migrations/新迁移目录/migration.sql

# 3. 重新构建并启动应用
docker compose up -d --build

# 4. 验证
curl http://localhost:3000/api/health
```

## 📝 注意事项

1. **.env 文件不会被打包到镜像中**（安全考虑）
2. **数据库迁移必须在启动前完成**
3. **首次启动需等待 40 秒**（健康检查 start_period）
4. **生产环境建议使用反向代理**（Nginx/Caddy + SSL）
