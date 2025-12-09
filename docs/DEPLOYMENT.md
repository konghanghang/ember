# 🚀 Ember 部署指南

> 本文档描述如何使用 Docker Compose 部署 Ember MVP 版本

---

## 📋 前置要求

- Docker 20.10+
- Docker Compose 2.0+
- Emby Server 4.7+（已运行且可访问）

---

## 🔧 快速部署

### 1. 克隆代码

```bash
git clone https://github.com/yourusername/ember.git
cd ember
```

### 2. 配置环境变量

复制 `.env.example` 为 `.env` 并修改配置：

```bash
cp .env.example .env
nano .env  # 或使用其他编辑器
```

**必须修改的配置**：

```bash
# 数据库密码
POSTGRES_PASSWORD="your-secure-password"

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
```

### 3. 启动服务

```bash
# 构建并启动
docker compose up -d

# 查看日志
docker compose logs -f ember
```

### 4. 验证部署并修改密码

访问 http://localhost:3000，使用管理员账号登录：
- 用户名：`admin`
- 密码：`.env` 中配置的 `ADMIN_DEFAULT_PASSWORD`（默认为 `admin123`）

⚠️ **首次登录后必须立即修改密码**：
1. 登录后导航到"系统设置"页面
2. 找到"修改密码"区块
3. 填写当前密码和新密码
4. 点击"修改密码"保存

🔒 **安全提示**：
- 生产环境强烈建议在 `.env` 中配置 `ADMIN_DEFAULT_PASSWORD`
- 新密码长度至少 6 个字符
- 建议使用包含大小写字母、数字和特殊字符的强密码

---

## 📦 部署架构

```
┌─────────────────┐
│  Ember App      │  端口: 3000
│  (Next.js)      │
└────────┬────────┘
         │
         ├─ 连接 PostgreSQL (内部网络)
         └─ 调用 Emby API (外部)

┌─────────────────┐
│  PostgreSQL     │  端口: 5432
│  (数据库)       │
└─────────────────┘
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

**解决**：修改 `docker-compose.yml` 中的端口映射：

```yaml
services:
  ember:
    ports:
      - "8080:3000"  # 改为其他端口
```

### 4. 数据持久化

PostgreSQL 数据存储在 Docker 卷 `postgres_data` 中。

**备份数据库**：

```bash
docker compose exec postgres pg_dump -U postgres ember > backup.sql
```

**恢复数据库**：

```bash
cat backup.sql | docker compose exec -T postgres psql -U postgres ember
```

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
   - ⚠️ 登录页面不再显示默认密码提示，请妥善保管密码

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

## 🛠️ 维护操作

### 查看日志

```bash
# 实时日志
docker compose logs -f ember

# 最近 100 行
docker compose logs --tail=100 ember
```

### 重启服务

```bash
# 重启所有服务
docker compose restart

# 只重启 Ember
docker compose restart ember
```

### 更新应用

```bash
# 拉取最新代码
git pull

# 重新构建并启动
docker compose up -d --build

# 运行数据库迁移（如果有）
docker compose exec ember npx prisma migrate deploy
```

### 手动触发定时任务

```bash
# 方法 1：通过 API
curl http://localhost:3000/api/cron

# 方法 2：在容器中执行
docker compose exec ember node -e "require('./server.js')"
```

### 清理数据

```bash
# 停止并删除容器
docker compose down

# 删除数据卷（⚠️ 会丢失所有数据）
docker compose down -v

# 删除镜像
docker rmi ember-ember
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

预期响应：

```json
{
  "status": "ok",
  "timestamp": "2025-12-08T...",
  "database": "connected"
}
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

## 📚 相关文档

- [README.md](../README.md) - 项目概览
- [需求文档](./specs/requirements.md) - MVP 功能需求
- [测试清单](./testing-checklist.md) - 测试验收标准
- [技术决策](./tech-stack-decision.md) - 技术选型说明

---

## 🆘 获取帮助

- **GitHub Issues**: https://github.com/yourusername/ember/issues
- **讨论区**: https://github.com/yourusername/ember/discussions

---

**部署成功后，请立即修改默认密码并配置 Emby 连接！**
