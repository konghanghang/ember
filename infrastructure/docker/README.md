# Docker 部署指南

本目录包含 Ember 项目的 Docker 部署配置。

## 📦 快速开始

### 1. 准备环境变量

```bash
# 复制环境变量模板
cp .env.example .env

# 编辑 .env 文件，填入实际配置
vi .env
```

### 2. 启动服务

```bash
# 拉取最新镜像
docker-compose pull

# 启动所有服务
docker-compose up -d

# 查看日志
docker-compose logs -f
```

### 3. 验证服务

```bash
# 检查服务状态
docker-compose ps

# 测试 API 健康检查
curl http://localhost:8080/health

# 测试 Web 前端
curl http://localhost
```

## 🔧 配置说明

### 服务列表

| 服务 | 容器名称 | 端口 | 说明 |
|------|---------|------|------|
| postgres | ember-postgres | 5432 | PostgreSQL 数据库 |
| api | ember-api | 8080 | Go API 服务 |
| web | ember-web | 80 | Vue 3 前端 |
| bot | ember-bot | 8000 | Telegram Bot |

### 镜像版本

默认使用 `latest` 标签的镜像。如需使用特定版本：

```yaml
# 修改 docker-compose.yml
services:
  api:
    image: ghcr.io/konghanghang/ember-api:v1.0.0  # 指定版本
```

### 本地构建

如果你想从源码构建镜像而不是使用预构建镜像：

1. 编辑 `docker-compose.yml`
2. 注释掉 `image:` 行
3. 取消注释 `build:` 部分

```yaml
# 使用预构建的镜像（生产环境推荐）
# image: ghcr.io/konghanghang/ember-api:latest

# 如需本地构建，取消下面两行注释并注释上面的 image 行
build:
  context: ../../services/api
  dockerfile: Dockerfile
```

然后运行：

```bash
docker-compose build
docker-compose up -d
```

## 📂 数据持久化

### 数据卷

- `postgres_data` - PostgreSQL 数据库数据
- `api_logs` - API 服务日志

### 备份数据库

```bash
# 备份
docker exec ember-postgres pg_dump -U postgres ember > backup.sql

# 恢复
docker exec -i ember-postgres psql -U postgres ember < backup.sql
```

## 🔍 故障排查

### 查看日志

```bash
# 查看所有服务日志
docker-compose logs

# 查看特定服务日志
docker-compose logs api
docker-compose logs -f web  # 实时跟踪

# 查看最近 100 行
docker-compose logs --tail=100 bot
```

### 重启服务

```bash
# 重启所有服务
docker-compose restart

# 重启特定服务
docker-compose restart api
```

### 清理重建

```bash
# 停止并删除容器
docker-compose down

# 删除容器和数据卷（⚠️ 会删除数据库数据）
docker-compose down -v

# 重新启动
docker-compose up -d
```

## 🚀 生产环境建议

### 1. 使用固定版本的镜像

```yaml
image: ghcr.io/konghanghang/ember-api:v1.0.0  # 不要用 latest
```

### 2. 配置外部数据库

生产环境建议使用托管的 PostgreSQL 服务，而不是容器化数据库。

### 3. 使用 Nginx 反向代理

```bash
# 启用 nginx 服务
# 取消 docker-compose.yml 中 nginx 部分的注释
# 配置 SSL 证书
# 修改端口映射（80 -> nginx, 443 -> nginx）
```

### 4. 配置日志轮转

已在 docker-compose.yml 中配置：
- 单个日志文件最大 10MB
- 保留最近 3 个日志文件

### 5. 监控和告警

推荐集成：
- Prometheus + Grafana - 性能监控
- Sentry - 错误追踪
- Uptime Kuma - 服务可用性监控

## 📋 常用命令

```bash
# 查看服务状态
docker-compose ps

# 更新镜像
docker-compose pull
docker-compose up -d

# 查看资源使用
docker stats

# 进入容器
docker exec -it ember-api sh
docker exec -it ember-postgres psql -U postgres

# 查看网络
docker network ls
docker network inspect ember_ember-network
```

## 🔐 安全建议

1. **修改默认密码** - 更改 `.env` 中的所有密钥
2. **限制端口暴露** - 生产环境不暴露数据库端口
3. **使用 HTTPS** - 配置 SSL/TLS 证书
4. **定期更新** - 保持镜像和依赖最新
5. **最小权限** - 所有服务都使用非 root 用户运行

## 📚 相关文档

- [Docker 构建指南](../../docs/runbooks/DOCKER-BUILD-GUIDE.md)
- [系统架构文档](../../docs/SYSTEM-ARCHITECTURE.md)
- [项目 README](../../README.md)
