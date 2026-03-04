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
psql $DATABASE_URL -f infrastructure/database/20260215_01_create_playback_rankings.sql

# 方法 2：使用临时 postgres 容器
cat infrastructure/database/20260215_01_create_playback_rankings.sql | \
  docker run --rm -i postgres:16-alpine psql "$DATABASE_URL"
```

### 3. 初始化管理员账号

**⚠️ 重要：统一认证后的新方式**

管理员账号通过环境变量自动初始化，无需手动执行 SQL 脚本。

**配置方式**（在 `.env` 文件中）：

```bash
# 管理员账号配置
ADMIN_USERNAME=admin              # 管理员用户名（推荐保持 admin）
ADMIN_PASSWORD=你的强密码         # 管理员密码（必须修改！）

# 数据库迁移开关（首次部署需开启）
AUTO_MIGRATE=true
```

**初始化流程**：

```bash
# 1. 配置环境变量（.env 文件）
ADMIN_USERNAME=admin
ADMIN_PASSWORD=$(openssl rand -base64 16)  # 生成强密码
AUTO_MIGRATE=true

# 2. 启动应用（会自动执行）
docker compose up -d

# 3. 查看日志确认
docker compose logs ember | grep "管理员"
# 预期输出：✅ 默认管理员已创建：admin
```

**工作原理**：

1. 应用启动时检查是否存在 `role=admin` 的用户
2. 如果不存在，读取 `ADMIN_USERNAME` 和 `ADMIN_PASSWORD`
3. 使用 bcrypt 加密密码后插入数据库
4. 幂等设计：多次启动不会重复创建

**安全建议**：

- ✅ 首次部署前在 `.env` 中配置强密码
- ✅ 管理员创建成功后，可从 `.env` 移除 `ADMIN_PASSWORD`（可选）
- ✅ 即使忘记移除也无影响（幂等检查会跳过）
- ❌ 禁止使用弱密码（如 `admin123`、`password`）

**常见问题**：

Q: 如果忘记配置 `ADMIN_USERNAME` 或 `ADMIN_PASSWORD` 怎么办？
A: 应用会输出警告日志并跳过 admin 初始化，需要补充环境变量后重启。

Q: 多次重启会重复创建管理员吗？
A: 不会。代码检查到已存在 admin 用户会直接跳过。

Q: 如何重置管理员密码？
A: 需要手动执行 SQL 更新（见下方"重置管理员密码"章节）。

---

### 3.1 重置管理员密码（可选）

如果忘记管理员密码，可通过数据库手动重置：

```bash
# 1. 生成新的 bcrypt 密码哈希（使用 Node.js）
node -e "console.log(require('bcryptjs').hashSync('新密码', 10))"

# 2. 更新数据库
psql $DATABASE_URL -c "UPDATE users SET password='上一步生成的哈希' WHERE role='admin' AND username='admin'"
```

或使用 Python：

```bash
# 生成密码哈希
python3 -c "import bcrypt; print(bcrypt.hashpw(b'新密码', bcrypt.gensalt()).decode())"

# 更新数据库（同上）
```

### 3.2 关联管理员 Emby 账号（可选）

管理员账号初始化时不会创建 Emby 账号。如需使用媒体相关功能，需手动关联已有的 Emby 用户。

**步骤 1：获取 Emby 用户 ID**

方式 A — Emby 管理面板：

打开 Emby 后台 → 用户 → 点击目标用户 → 浏览器地址栏 URL 中 `userId=` 后的值即为 Emby 用户 ID。

方式 B — Emby API：

```bash
curl "http://你的Emby地址:8096/emby/Users?api_key=你的API_KEY"
# 在返回的 JSON 数组中，找到 Name 匹配管理员用户名的记录，取 Id 字段
```

**步骤 2：更新数据库**

```sql
UPDATE users SET "embyId" = '你的Emby用户ID' WHERE username = '你的管理员用户名';
```

**验证**：登录管理后台，检查用户管理页面中管理员的 Emby ID 列是否已显示。

---

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

### 定时任务配置

**✅ 应用已内置定时任务调度器，无需额外配置！**

应用启动时会自动初始化定时任务，每天凌晨 2:00（Asia/Shanghai 时区）执行用户过期检查。

#### 工作原理

应用使用内置的 `node-cron` 调度器，在服务器启动时自动初始化：

```
应用启动 → instrumentation.ts → lib/scheduler.ts → 定时任务运行
```

**优点**：
- ✅ 自包含，无需外部配置
- ✅ Docker 容器启动即生效
- ✅ 代码即配置，便于维护
- ✅ 支持多实例部署（每个实例独立运行）

#### 手动触发（测试用）

```bash
# 如果未设置 CRON_SECRET
curl http://localhost:3000/api/cron

# 如果设置了 CRON_SECRET（推荐）
curl -H "Authorization: Bearer your-cron-secret-key" http://localhost:3000/api/cron

# 查看格式化的 JSON 输出
curl -s http://localhost:3000/api/cron | jq .

# 预期响应示例
{
  "success": true,
  "message": "已处理 0 个过期用户，成功禁用 0 个",
  "data": {
    "disabledCount": 0,
    "totalExpired": 0,
    "errors": []
  }
}
```

**验证步骤**：

1. **检查应用是否运行**：
   ```bash
   curl http://localhost:3000/api/health
   # 预期返回: {"status":"ok", ...}
   ```

2. **手动触发定时任务**：
   ```bash
   curl http://localhost:3000/api/cron
   ```

3. **查看应用日志**：
   ```bash
   docker compose logs -f ember | grep -E "(Scheduler|Cron)"
   ```

#### 查看日志

```bash
# Docker 部署
docker compose logs -f ember | grep Scheduler

# 查看定时任务执行日志
docker compose logs -f ember | grep "Cron"
```

#### 修改执行时间

通过环境变量配置（推荐）：

```bash
# 在 .env 文件中设置
CRON_SCHEDULE="0 2 * * *"      # 每天凌晨 2:00（默认）
CRON_TIMEZONE="Asia/Shanghai"  # 中国标准时间（默认）
```

**常用 Cron 表达式示例**：

```bash
# 每天执行一次
CRON_SCHEDULE="0 2 * * *"     # 每天凌晨 2:00
CRON_SCHEDULE="0 12 * * *"    # 每天中午 12:00
CRON_SCHEDULE="0 0 * * *"     # 每天午夜 0:00

# 每隔几小时执行
CRON_SCHEDULE="0 */6 * * *"   # 每 6 小时执行一次（0:00, 6:00, 12:00, 18:00）
CRON_SCHEDULE="0 */12 * * *"  # 每 12 小时执行一次

# 每周执行
CRON_SCHEDULE="0 2 * * 0"     # 每周日凌晨 2:00
CRON_SCHEDULE="0 2 * * 1"     # 每周一凌晨 2:00

# 测试用（高频执行）
CRON_SCHEDULE="*/30 * * * *"  # 每 30 分钟执行一次
CRON_SCHEDULE="*/5 * * * *"   # 每 5 分钟执行一次
```

**时区配置示例**：

```bash
CRON_TIMEZONE="Asia/Shanghai"      # 中国（UTC+8）
CRON_TIMEZONE="America/New_York"   # 美国东部
CRON_TIMEZONE="Europe/London"      # 英国
CRON_TIMEZONE="UTC"                # 协调世界时
```

**Cron 表达式格式**：`分 时 日 月 周`（5 个字段）

| 字段 | 允许值 | 特殊字符 |
|------|--------|----------|
| 分   | 0-59   | `*` `,` `-` `/` |
| 时   | 0-23   | `*` `,` `-` `/` |
| 日   | 1-31   | `*` `,` `-` `/` |
| 月   | 1-12   | `*` `,` `-` `/` |
| 周   | 0-7    | `*` `,` `-` `/` (0 和 7 都表示周日) |

#### Vercel 部署

Vercel 平台会使用 `vercel.json` 中的配置：

```json
{
  "crons": [{
    "path": "/api/cron",
    "schedule": "0 2 * * *"
  }]
}
```

**注意**：Vercel 部署时，应用内部的调度器和 Vercel Cron 会同时生效，不会冲突（两者都是调用同一个函数）。

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
# CI/CD 使用指南

## 📋 概述

Ember 使用 GitHub Actions 实现自动化的持续集成和持续部署。

## 🔄 CI - 持续集成

**触发条件**：
- Push 到 `master`、`main` 或 `develop` 分支
- 创建 Pull Request 到 `master` 或 `main` 分支

**智能跳过**：
以下文件修改时不会触发 CI（节省构建时间）：
- 📝 所有 Markdown 文件（`*.md`）
- 📚 文档目录（`docs/**`）
- 📄 LICENSE、.gitignore 等配置文件

> **示例**：只修改 `README.md` 或 `docs/` 目录时，CI 会自动跳过

**验证内容**：
1. ✅ Prisma Schema 验证
2. ✅ 生成 Prisma Client
3. ✅ 代码风格检查（ESLint）
4. ✅ Next.js 编译构建

**查看结果**：
- GitHub 仓库 → Actions 标签页
- Pull Request 页面会显示检查状态

---

## 🧪 Master 镜像 - 测试环境

**触发条件**：
- Push 代码到 `master` 分支（代码修改时）
- 与 CI 使用相同的 `paths-ignore` 规则

**构建内容**：
自动构建并推送两个测试镜像标签：
```bash
# 最新的 master 分支镜像（总是覆盖）
ghcr.io/konghanghang/ember:master

# 特定 commit 的镜像（可回溯）
ghcr.io/konghanghang/ember:master-abc1234
```

**典型开发流程**：

```bash
# 1. 开发并推送代码
git add .
git commit -m "feat: 添加新功能"
git push origin master

# 2. GitHub Actions 自动执行（并行）：
#    ✅ CI 验证（2-3 分钟）
#    ✅ 构建 master 镜像（3-5 分钟）

# 3. 在测试环境拉取最新 master 镜像
docker pull ghcr.io/konghanghang/ember:master

# 4. 测试验证
docker run -d -p 3000:3000 --env-file .env \
  ghcr.io/konghanghang/ember:master

# 5. 测试通过后打生产 tag
git tag v1.0.0
git push origin v1.0.0
```

**使用场景**：
- 🧪 测试环境部署最新开发版本
- 🔄 持续集成测试
- 🐛 快速验证 bug 修复
- 📋 回滚到特定 commit（使用 `master-<sha>` 标签）

**注意事项**：
- ⚠️ `master` 镜像仅用于测试，不要用于生产环境
- ⚠️ `master` 镜像会被每次 push 覆盖
- ✅ 如需回滚到特定版本，使用 `master-<commit-sha>` 标签

---

## 🚀 Release - 发布生产版本

### 发布新版本

```bash
# 1. 确保代码已提交并推送到 master
git checkout master
git pull

# 2. 创建并推送 tag（遵循语义化版本）
git tag v1.0.0
git push origin v1.0.0

# 3. GitHub Actions 自动执行：
#    - 构建 Docker 镜像
#    - 推送到 ghcr.io
#    - 生成 Release Notes
#    - 创建 GitHub Release
```

### 版本号规范

遵循 [语义化版本](https://semver.org/lang/zh-CN/) 规范：

```
v<主版本>.<次版本>.<修订版本>

示例：
- v1.0.0  - 首个正式版本
- v1.1.0  - 新增功能（向后兼容）
- v1.1.1  - Bug 修复
- v2.0.0  - 破坏性更新（不兼容旧版本）
```

### Docker 镜像标签策略

项目提供两类镜像：**测试镜像**和**生产镜像**

#### 测试镜像（Master 分支）

```bash
# 最新的 master 分支（每次 push master 自动更新）
ghcr.io/konghanghang/ember:master

# 特定 commit 的测试镜像（可回溯）
ghcr.io/konghanghang/ember:master-abc1234
```

#### 生产镜像（Tag 发布）

每次发布会生成以下标签：

```bash
# 完整版本号
ghcr.io/konghanghang/ember:v1.2.3
ghcr.io/konghanghang/ember:1.2.3

# 主版本号 + 次版本号
ghcr.io/konghanghang/ember:1.2

# 主版本号
ghcr.io/konghanghang/ember:1

# 最新生产版本
ghcr.io/konghanghang/ember:latest
```

#### 使用场景对比

| 环境 | 推荐镜像 | 更新频率 | 稳定性 |
|------|---------|---------|--------|
| 开发/测试 | `:master` | 每次 push | 不稳定 |
| 预发布 | `:v1.2.3` | 手动发布 | 稳定 |
| 生产环境 | `:v1.2.3` | 手动发布 | 稳定 |

**使用示例**：

```bash
# 测试环境：拉取最新开发版本
docker pull ghcr.io/konghanghang/ember:master

# 生产环境：拉取最新稳定版本
docker pull ghcr.io/konghanghang/ember:latest

# 拉取特定版本（推荐）
docker pull ghcr.io/<用户名>/ember:v1.2.3

# 使用特定版本部署
docker run -d -p 3000:3000 ghcr.io/<用户名>/ember:v1.2.3
```

---

## 📦 使用发布的镜像

### 方式一：直接使用 Docker

```bash
# 使用 .env 文件
docker run -d \
  --name ember-app \
  -p 3000:3000 \
  --env-file .env \
  ghcr.io/<用户名>/ember:latest
```

### 方式二：更新 docker-compose.yaml

```yaml
services:
  ember:
    image: ghcr.io/<用户名>/ember:v1.2.3  # 使用具体版本
    # image: ghcr.io/<用户名>/ember:latest  # 或使用最新版本
    container_name: ember-app
    restart: unless-stopped
    ports:
      - "3000:3000"
    env_file:
      - .env
```

```bash
# 拉取并启动
docker compose pull
docker compose up -d
```

---

## ⚙️ CI 触发规则详解

### 会触发 CI 的修改

以下文件修改会触发 CI 验证：

```bash
✅ 代码文件
app/**/*.{ts,tsx,js,jsx}
src/**/*.{ts,tsx,js,jsx}
lib/**/*.{ts,tsx,js,jsx}
components/**/*.{ts,tsx,js,jsx}

✅ 配置文件
package.json
package-lock.json
tsconfig.json
next.config.ts
tailwind.config.ts

✅ 数据库相关
prisma/schema.prisma
prisma.config.ts
prisma/migrations/**

✅ Docker 相关
Dockerfile
docker-compose.yaml
docker-compose.local.yml

✅ CI/CD 配置
.github/workflows/*.yml
```

### 不会触发 CI 的修改

以下文件修改会跳过 CI（节省资源）：

```bash
❌ 文档文件
README.md
docs/**/*.md
DOCKER_QUICKSTART.md
*.md

❌ 配置文件
LICENSE
.gitignore
.prettierrc
.editorconfig
```

### 示例场景

```bash
# 场景 1：只修改 README.md
git commit -m "docs: 更新安装说明"
git push
# 结果：CI 跳过 ⏭️

# 场景 2：修改代码和文档
git commit -m "feat: 添加新功能并更新文档"
git push
# 结果：CI 触发 ✅（因为包含代码修改）

# 场景 3：只修改 Dockerfile
git commit -m "chore: 优化 Docker 构建"
git push
# 结果：CI 触发 ✅

# 场景 4：批量更新文档
git commit -m "docs: 完善部署文档"
git push
# 结果：CI 跳过 ⏭️（如果只修改了 docs/*.md）
```

---

## 🐛 故障排查

### CI 构建失败

1. **查看错误日志**
   - GitHub 仓库 → Actions → 点击失败的 workflow

2. **常见问题**
   - 代码风格错误：运行 `npm run lint` 本地检查
   - 编译错误：运行 `npm run build` 本地验证
   - Prisma Schema 错误：运行 `npx prisma validate`

### Release 发布失败

1. **检查权限**
   - 确保 GitHub Actions 有 `packages: write` 权限

2. **镜像推送失败**
   - 检查 tag 格式是否正确（必须是 `v*.*.*`）
   - 查看 Actions 日志中的具体错误

3. **Release 创建失败**
   - 确保 tag 不重复
   - 检查 `GITHUB_TOKEN` 权限

### 本地测试 Docker 构建

```bash
# 模拟 CI 环境构建镜像
docker build -t ember:test .

# 测试运行
docker run -d -p 3000:3000 --env-file .env ember:test
```

---

## 📝 最佳实践

### 1. 发布前检查

```bash
# 确保 CI 通过
git push origin master
# 等待 GitHub Actions CI 检查通过

# 本地验证
npm run lint
npm run build

# 创建 tag
git tag v1.0.0
git push origin v1.0.0
```

### 2. Commit 消息规范

Release Notes 基于 commit 消息生成，建议遵循规范：

```bash
# 功能
feat: 添加用户导出功能

# 修复
fix: 修复邀请码过期判断错误

# 文档
docs: 更新部署指南

# 性能优化
perf: 优化数据库查询性能

# 重构
refactor: 重构认证模块
```

### 3. 版本发布节奏

- **补丁版本（v1.0.x）**：Bug 修复，随时发布
- **次版本（v1.x.0）**：新功能，1-2 周发布
- **主版本（vx.0.0）**：重大更新，谨慎发布

---

## 🔐 GitHub Packages 访问

### 拉取公开镜像

无需登录即可拉取公开镜像。

### 拉取私有镜像

```bash
# 1. 创建 Personal Access Token
# GitHub → Settings → Developer settings → Personal access tokens
# 权限：read:packages

# 2. 登录
echo $GITHUB_TOKEN | docker login ghcr.io -u <用户名> --password-stdin

# 3. 拉取镜像
docker pull ghcr.io/<用户名>/ember:latest
```

---

## 📚 相关文档

- [GitHub Actions 文档](https://docs.github.com/en/actions)
- [GitHub Packages 文档](https://docs.github.com/en/packages)
- [Docker Compose 文档](https://docs.docker.com/compose/)
- [语义化版本规范](https://semver.org/lang/zh-CN/)
