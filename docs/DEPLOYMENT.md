# 🚀 Ember 部署指南

> 完整的 Docker 部署文档，涵盖快速开始、生产部署和故障排查

---

## 📋 前置要求

- Docker 20.10+
- Docker Compose 2.0+
- Emby Server 4.7+（已运行且可访问）

---

## 📦 部署模式选择

Ember 提供三种 Docker 部署模式：

| 模式 | 配置文件 | 适用场景 | 镜像来源 |
|------|---------|---------|---------|
| **模式零：预构建镜像** | 修改 compose 文件 | 快速部署/测试 | GitHub Packages |
| **模式一：生产部署** | `docker-compose.yaml` | 生产环境 | 本地构建 |
| **模式二：本地开发** | `docker-compose.local.yml` | 本地开发 | 本地构建 |

---

## 🎯 模式零：使用预构建镜像（最快）

**直接使用 GitHub 自动构建的镜像，无需本地构建**

### 快速开始

```bash
# 1. 克隆代码
git clone https://github.com/yourusername/ember.git
cd ember

# 2. 配置环境变量
cp .env.example .env
nano .env  # 修改必要配置（见下方）

# 3. 修改 docker-compose.yaml，使用预构建镜像
# 将 build 部分替换为：
#   image: ghcr.io/konghanghang/ember:latest  # 生产稳定版
#   或
#   image: ghcr.io/konghanghang/ember:master  # 最新开发版

# 4. 拉取并启动
docker compose pull
docker compose up -d

# 5. 查看日志
docker compose logs -f ember
```

**镜像标签说明**：
- `:latest` - 最新稳定版本（生产推荐）
- `:v1.0.0` - 特定版本（生产推荐）
- `:master` - 最新开发版本（测试用）

**优点**：
- ✅ 无需本地构建（节省 3-5 分钟）
- ✅ 镜像经过 CI 验证
- ✅ 适合快速部署和测试

---

## 🚀 模式一：生产部署（推荐）

**使用远程 PostgreSQL 数据库**

### 1. 配置环境变量

复制 `.env.example` 为 `.env` 并修改配置：

```bash
cp .env.example .env
nano .env
```

**必须修改的配置**：

```bash
# 数据库连接（远程 PostgreSQL）
DATABASE_URL="postgresql://用户名:密码@主机:端口/数据库名"

# JWT 密钥（至少 32 个字符）
JWT_SECRET="$(openssl rand -base64 32)"

# 管理员初始密码（强烈建议修改）
ADMIN_DEFAULT_PASSWORD="your-secure-admin-password"

# Emby 服务器配置
EMBY_URL="https://your-emby-server.com"
NEXT_PUBLIC_EMBY_URL="https://your-emby-server.com"
EMBY_API_KEY="your-emby-api-key"
```

**可选配置**：

```bash
# Cron 任务密钥（可选）
CRON_SECRET="$(openssl rand -base64 32)"

# 应用访问地址
NEXT_PUBLIC_APP_URL="https://your-domain.com"

# 环境
NODE_ENV=production

# MoviePilot 集成（可选，用于自动订阅影视资源）
# 如果不配置，订阅功能仍可使用，但不会自动调用 MoviePilot API
MOVIEPILOT_URL="http://your-moviepilot-server:3001"
MOVIEPILOT_USERNAME="admin"
MOVIEPILOT_PASSWORD="your-moviepilot-password"
```

**MoviePilot 配置说明**：

- **作用**：审核通过订阅时，自动调用 MoviePilot API 创建订阅
- **可选性**：如果不配置，用户仍可提交订阅、管理员仍可审核，但不会调用 MP API
- **错误处理**：MP API 调用失败时，订阅状态仍为"已批准"，但会显示同步失败信息
- **验证配置**：登录管理后台，查看订阅管理页面的同步状态

### 2. 数据库迁移

**首次部署或更新时需要执行**

```bash
# 方法 1：使用 psql（推荐）
psql $DATABASE_URL -f prisma/migrations/20251207010855_ember/migration.sql

# 方法 2：使用临时 postgres 容器
cat prisma/migrations/20251207010855_ember/migration.sql | \
  docker run --rm -i postgres:16-alpine psql "$DATABASE_URL"
```

### 3. 初始化管理员账号

```bash
# 执行初始化脚本
psql $DATABASE_URL -f prisma/migrations/init-admin.sql

# 或使用自定义账号
node scripts/create-admin.js admin MyPass123 | psql $DATABASE_URL

# 默认账号：admin / admin123
```

### 4. 启动应用

```bash
# 构建并启动
docker compose up -d --build

# 查看日志
docker compose logs -f ember
```

### 5. 验证部署并修改密码

访问 http://localhost:3000，使用管理员账号登录：
- 用户名：`admin`
- 密码：`.env` 中配置的 `ADMIN_DEFAULT_PASSWORD`（默认为 `admin123`）

⚠️ **首次登录后必须立即修改密码**：
1. 登录后导航到"系统设置"页面
2. 找到"修改密码"区块
3. 填写当前密码和新密码
4. 点击"修改密码"保存

---

## 🏠 模式二：本地开发

**包含本地 PostgreSQL 数据库**

```bash
# 1. 配置 .env 文件
cp .env.example .env
echo "POSTGRES_PASSWORD=your-secure-password" >> .env

# 2. 启动所有服务（应用 + 数据库）
docker compose -f docker-compose.local.yml up -d --build

# 3. 执行数据库迁移
cat prisma/migrations/20251207010855_ember/migration.sql \
    prisma/migrations/init-admin.sql | \
  docker compose -f docker-compose.local.yml exec -T postgres \
    psql -U postgres -d ember

# 4. 查看日志
docker compose -f docker-compose.local.yml logs -f
```

访问：http://localhost:3000

> **注意**：本地模式会在 Docker 中启动 PostgreSQL，数据存储在 `postgres_data` volume 中。

---

## 📦 部署架构

```
┌─────────────────┐
│  Ember App      │  端口: 3000
│  (Next.js)      │  镜像大小: 351MB
└────────┬────────┘
         │
         ├─ 连接 PostgreSQL (内部网络或远程)
         └─ 调用 Emby API (外部)

┌─────────────────┐
│  PostgreSQL     │  端口: 5432
│  (数据库)       │  (可选，本地开发模式)
└─────────────────┘
```

**镜像特性**：
- 351MB 纯运行时（多阶段构建，standalone 输出）
- 只包含 Prisma Client，无迁移工具
- 非 root 用户运行（nextjs:1001）
- 内置健康检查

---

## 🔧 常用命令

### 生产模式命令

```bash
# 查看容器状态
docker compose ps

# 查看实时日志
docker compose logs -f ember

# 最近 100 行日志
docker compose logs --tail=100 ember

# 重启服务
docker compose restart

# 只重启 Ember
docker compose restart ember

# 停止服务
docker compose down

# 进入容器 Shell（调试用）
docker compose exec ember sh

# 重新构建镜像（无缓存）
docker compose build --no-cache

# 更新应用
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

### 手动触发定时任务

```bash
# 方法 1：通过 API
curl http://localhost:3000/api/cron

# 方法 2：在容器中执行
docker compose exec ember node -e "require('./server.js')"
```

### 数据库备份和恢复

```bash
# 备份数据库
docker compose exec postgres pg_dump -U postgres ember > backup.sql

# 恢复数据库
cat backup.sql | docker compose exec -T postgres psql -U postgres ember
```

---

## 🔍 常见问题

### 1. 数据库迁移失败

**现象**：`docker compose logs ember` 显示 Prisma migrate 失败

**解决**：

```bash
# 手动运行迁移
docker compose exec ember npx prisma migrate deploy

# 创建初始管理员
docker compose exec ember npx prisma db seed
```

### 2. Emby 连接失败

**现象**：注册用户时提示 "Emby 用户创建失败"

**检查清单**：
- [ ] `EMBY_URL` 配置正确（不要以 / 结尾）
- [ ] `EMBY_API_KEY` 有效（在 Emby 后台检查）
- [ ] Emby 服务器可从 Docker 容器访问（网络连通性）

**测试连接**：

```bash
# 进入容器
docker compose exec ember sh

# 测试 Emby API
curl -H "X-Emby-Token: $EMBY_API_KEY" "$EMBY_URL/Users"
```

### 3. 端口冲突

**现象**：`docker compose up` 失败，提示端口被占用

**解决**：修改 `docker-compose.yaml` 中的端口映射：

```yaml
services:
  ember:
    ports:
      - "8080:3000"  # 改为其他端口
```

### 4. 容器无法启动

```bash
# 查看详细日志
docker compose logs ember

# 检查环境变量配置
docker compose config

# 重新构建（无缓存）
docker compose build --no-cache
```

### 5. 数据库连接失败

1. 检查 .env 文件中的 DATABASE_URL 格式
2. 确认数据库服务器可访问：`ping 数据库主机`
3. 检查防火墙规则
4. 测试网络连通性：`docker compose exec ember ping 数据库主机`

### 6. 健康检查失败

等待 40 秒启动时间后再检查：

```bash
# 手动测试健康检查
curl http://localhost:3000/api/health

# 预期响应
{
  "status": "ok",
  "timestamp": "2025-12-13T...",
  "database": "connected"
}

# 查看应用日志是否有错误
docker compose logs -f ember
```

### 7. 应用运行但无法访问

1. 检查端口映射：`docker compose ps`
2. 确认防火墙未阻止 3000 端口
3. 验证 NEXT_PUBLIC_APP_URL 配置

---

## 🔐 安全建议

### 生产环境必做

1. **管理员密码管理**

   **部署前配置**（推荐）：
   ```bash
   # .env 文件
   ADMIN_DEFAULT_PASSWORD="$(openssl rand -base64 16)"
   ```

   **首次登录后修改**（必须）：
   1. 登录管理后台（用户名：`admin`）
   2. 导航到"系统设置" → "修改密码"
   3. 填写当前密码和新密码
   4. 点击"修改密码"

   **密码策略**：
   - ✅ 最小长度：6 个字符
   - ✅ 推荐：12+ 个字符，包含大小写字母、数字、特殊字符
   - ❌ 禁止：弱密码如 `123456`、`password`、`admin`

   **重要提示**：
   - ⚠️ 如果未配置 `ADMIN_DEFAULT_PASSWORD`，系统将使用默认密码 `admin123`
   - ⚠️ 默认密码仅供开发测试使用，生产环境**必须修改**

2. **数据库密码安全**
   - 使用强密码（至少 16 位）
   - 不要使用默认密码 `password`
   - 定期更换数据库密码

3. **使用 HTTPS**
   - 配置 Nginx/Caddy 反向代理
   - 启用 SSL 证书（Let's Encrypt）

4. **配置防火墙**
   ```bash
   # 只允许 Web 端口
   ufw allow 80/tcp
   ufw allow 443/tcp
   ufw enable
   ```

5. **定期备份数据库**
   ```bash
   # 添加到 crontab
   0 2 * * * docker compose -f /path/to/ember/docker-compose.yml exec postgres pg_dump -U postgres ember > /backup/ember-$(date +\%Y\%m\%d).sql
   ```

### 可选增强

- 限制 PostgreSQL 端口暴露（只保留内部网络）
- 设置 `CRON_SECRET` 保护定时任务端点
- 集成监控工具（如 Prometheus + Grafana）

---

## 🏭 生产环境最佳实践

### 1. 环境变量管理

**不推荐**：将敏感信息直接写在 .env 文件中

**推荐方案**：
- 使用 Docker Secrets（Docker Swarm）
- 使用外部密钥管理服务（AWS Secrets Manager、HashiCorp Vault）
- 使用环境变量注入（Kubernetes ConfigMap/Secret）

### 2. 数据库迁移策略

**最佳实践**：
- ✅ 在部署流水线中独立执行迁移
- ✅ 迁移完成后再更新应用容器
- ❌ 不要在容器启动时自动运行迁移（避免竞态条件）

### 3. 反向代理和 SSL

推荐使用 Nginx 或 Caddy：

```yaml
# docker-compose.prod.yml 示例
services:
  nginx:
    image: nginx:alpine
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf
      - ./ssl:/etc/nginx/ssl
    depends_on:
      - ember

  ember:
    # ... 应用配置
    expose:
      - "3000"  # 只暴露给内部网络
```

### 4. 监控和日志

**日志收集**：
- 使用 ELK Stack（Elasticsearch、Logstash、Kibana）
- 或 Loki + Grafana
- 配置日志轮转避免磁盘占满

**性能监控**：
- 应用性能：New Relic、Datadog
- 容器监控：Prometheus + Grafana
- 数据库监控：pg_stat_statements

### 5. 资源限制

```yaml
# docker-compose.yaml
services:
  ember:
    # ... 其他配置
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G
        reservations:
          cpus: '1'
          memory: 1G
```

---

## 📊 监控

### 健康检查

```bash
# 检查服务状态
docker compose ps

# 应用健康检查
curl http://localhost:3000/api/health
```

### 性能监控

**数据库连接数**：

```bash
docker compose exec postgres psql -U postgres -c "SELECT count(*) FROM pg_stat_activity;"
```

**内存使用**：

```bash
docker stats ember-app
```

---

## 🐛 故障排查

### 日志位置

- **应用日志**：`docker compose logs ember`
- **数据库日志**：`docker compose logs postgres`
- **Prisma 日志**：容器内 `/app/.next/trace`

### 常见错误

| 错误信息 | 原因 | 解决方案 |
|---------|------|---------|
| `P1001: Can't reach database` | 数据库未启动或连接失败 | 检查 `DATABASE_URL` 配置 |
| `Emby user creation failed` | Emby API 调用失败 | 检查 `EMBY_URL` 和 `EMBY_API_KEY` |
| `JWT malformed` | JWT_SECRET 配置错误 | 确保至少 32 个字符 |
| `Permission denied` | 文件权限问题 | 检查 Docker 卷权限 |

### 进入容器调试

```bash
# 进入 Ember 容器
docker compose exec ember sh

# 进入 PostgreSQL 容器
docker compose exec postgres psql -U postgres ember
```

---

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

---

## 📚 相关文档

- [README.md](../README.md) - 项目概览
- [需求文档](./specs/requirements.md) - MVP 功能需求
- [设计文档](./specs/design.md) - 详细设计
- [测试清单](./testing-checklist.md) - 测试验收标准
- [开发指南](./development-guide.md) - 开发环境搭建和规范
- [CI/CD 指南](./cicd-guide.md) - 自动化部署流程

---

## 🆘 获取帮助

- **GitHub Issues**: https://github.com/yourusername/ember/issues
- **讨论区**: https://github.com/yourusername/ember/discussions

---

## 📝 注意事项

1. **.env 文件不会被打包到镜像中**（安全考虑）
2. **数据库迁移必须在启动前完成**
3. **首次启动需等待 40 秒**（健康检查 start_period）
4. **生产环境强烈建议使用反向代理**（Nginx/Caddy + SSL）
5. **定期备份数据库**，避免数据丢失
6. **监控磁盘空间**，配置日志轮转

---

**部署成功后，请立即修改默认密码并配置 Emby 连接！**

**文档更新日期**: 2025-12-13
