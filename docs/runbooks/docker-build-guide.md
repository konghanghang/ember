# Docker 构建和发布指南

本文档说明了 Ember 项目的 Docker 镜像构建和发布流程。

## 📦 镜像架构

Ember 采用 **Monorepo 微服务架构**，为每个服务独立构建 Docker 镜像：

| 服务 | 镜像名称 | 技术栈 | 说明 |
|------|----------|--------|------|
| API | `ghcr.io/konghanghang/ember-api` | Go 1.23 + Gin | 后端 API 服务 |
| Web | `ghcr.io/konghanghang/ember-web` | Vue 3 + Vite + nginx | 前端 SPA |
| Bot | `ghcr.io/konghanghang/ember-bot` | Python 3.11 | Telegram Bot |

## 🏗️ Dockerfile 说明

### API 服务 (`services/api/Dockerfile`)

**特点**：
- 多阶段构建（builder + runtime）
- 使用 `golang:1.23-alpine` 构建，`alpine:latest` 运行
- 非 root 用户运行（ember:ember）
- 健康检查：`/health` 端点
- 端口：8080

**构建产物**：单个二进制文件 `ember`

### Web 服务 (`services/web/Dockerfile`)

**特点**：
- 多阶段构建（builder + nginx）
- 使用 `node:20-alpine` 构建，`nginx:alpine` 运行
- Vite 构建生成静态文件，nginx 服务
- 自定义 nginx 配置（SPA 路由支持）
- 健康检查：`/` 端点
- 端口：80

**构建产物**：静态文件 (HTML, CSS, JS)

### Bot 服务 (`services/bot/Dockerfile`)

**特点**：
- 多阶段构建（builder + runtime）
- 使用 `python:3.11-slim` 构建和运行
- 非 root 用户运行（ember:ember）
- pip 依赖安装在用户目录
- 健康检查：`/health` 端点
- 端口：8000

**构建产物**：Python 应用 + 依赖

## 🚀 GitHub Actions 工作流

### 测试流程 (`test.yml`)

**触发条件**：
- Push 到 `master`, `main`, `develop` 分支
- Pull Request 到 `master`, `main` 分支
- 忽略文档、README 等变更

**执行内容**：
- `test-api`: Go 编译验证（`go vet`, `go build`）
- `test-web`: Vue 构建验证（`npm ci`, `npm run build`）
- `test-bot`: Python 语法检查（`py_compile`）

### 镜像构建流程 (`build-*.yml`)

**触发条件**：
1. **预览构建**：Push 到 `pre_release` 分支
2. **正式构建**：Push tag `v*`（例如 `v1.0.0`）

**执行内容**：
- 为每个服务独立构建 Docker 镜像
- 推送到 GitHub Container Registry (GHCR)
- 使用 GitHub Actions Cache 加速构建

**生成的镜像 tag**：

| 触发方式 | 镜像 Tag | 架构支持 | 说明 |
|---------|----------|---------|------|
| Push `pre_release` 分支 | `preview`<br>`preview-{sha}` | `amd64` | 测试版本，仅 amd64 加快构建 |
| Push tag `v1.0.0` | `v1.0.0`<br>`latest` | `amd64` + `arm64` | 正式版本，完整多架构支持 |

### Release 流程 (`create-release.yml`)

**触发条件**：
- Push tag `v*`（例如 `v1.0.0`）

**执行内容**：
1. 提取版本信息（稳定版 / 预发布版）
2. 生成 Changelog（基于 commits）
3. 创建 Draft Release（草稿状态）
4. 包含所有服务的镜像信息和升级说明

**注意**：Release 创建为草稿，需要手动编辑后发布。

## 📝 发布流程

### 1. 本地测试

```bash
# 测试 API 构建
cd services/api
go build -v ./...

# 测试 Web 构建
cd services/web
npm ci
npm run build

# 测试 Bot 构建
cd services/bot
python -m py_compile main.py
```

### 2. 构建预览版本（测试用）

```bash
# 1. 切换到 pre_release 分支
git checkout pre_release

# 2. 合并 master 分支的最新代码
git merge master

# 3. 推送到远程，触发预览镜像构建
git push origin pre_release

# 4. GitHub Actions 会自动构建并推送预览镜像
# - ghcr.io/konghanghang/ember-api:preview
# - ghcr.io/konghanghang/ember-web:preview
# - ghcr.io/konghanghang/ember-bot:preview
```

### 3. 测试预览镜像

```bash
# 使用预览镜像部署测试
cd infrastructure/docker

# 修改 docker-compose.yml，使用 preview tag
# image: ghcr.io/konghanghang/ember-api:preview

docker-compose pull
docker-compose up -d

# 进行功能测试...
```

### 4. 发布正式版本

```bash
# 1. 确认测试通过，切回 master 分支
git checkout master
git pull

# 2. 创建并推送 tag
git tag v1.0.0
git push origin v1.0.0
```

### 5. GitHub Actions 自动执行

- ✅ 构建 3 个服务的正式 Docker 镜像
- ✅ 推送镜像到 GHCR（tag: v1.0.0 + latest）
- ✅ 创建 Draft Release

### 6. 手动发布 Release

1. 访问 GitHub Releases 页面
2. 编辑 Draft Release
3. 根据 Commits 列表完善 Release Notes
4. 点击 "Publish release"

## 🔧 本地构建镜像

### API 服务

```bash
cd services/api
docker build -t ember-api:dev .
docker run -p 8080:8080 ember-api:dev
```

### Web 服务

```bash
cd services/web
docker build -t ember-web:dev .
docker run -p 80:80 ember-web:dev
```

### Bot 服务

```bash
cd services/bot
docker build -t ember-bot:dev .
docker run -p 8000:8000 ember-bot:dev
```

## 📦 使用已发布的镜像

### 使用 docker-compose 部署

**推荐方式**：使用预构建镜像一键部署

```bash
# 1. 进入 docker 目录
cd infrastructure/docker

# 2. 配置环境变量
cp .env.example .env
vi .env  # 填入实际配置

# 3. 拉取最新镜像并启动
docker-compose pull
docker-compose up -d

# 4. 查看服务状态
docker-compose ps
docker-compose logs -f
```

**详细说明**：参考 `infrastructure/docker/README.md`

### 手动拉取镜像

```bash
# 拉取最新版本
docker pull ghcr.io/konghanghang/ember-api:latest
docker pull ghcr.io/konghanghang/ember-web:latest
docker pull ghcr.io/konghanghang/ember-bot:latest

# 拉取指定版本
docker pull ghcr.io/konghanghang/ember-api:v1.0.0
docker pull ghcr.io/konghanghang/ember-web:v1.0.0
docker pull ghcr.io/konghanghang/ember-bot:v1.0.0
```

## ⚠️ 常见问题

### Q1: 构建失败，找不到文件

**原因**：构建上下文和 Dockerfile 路径配置错误。

**解决**：确保 workflows 中的配置正确：
```yaml
context: ./services/api
file: ./services/api/Dockerfile
```

### Q2: 健康检查失败

**原因**：服务启动时间过长或健康检查路径错误。

**解决**：
- 检查 `start-period` 是否足够（API: 5s, Web: 10s, Bot: 10s）
- 确认健康检查路径是否正确

### Q3: 多架构构建失败

**原因**：某些依赖不支持 ARM 架构。

**解决**：修改 workflows，只构建 `linux/amd64`：
```yaml
platforms: linux/amd64
```

### Q4: Web 前端路由 404

**原因**：nginx 未配置 SPA 路由支持。

**解决**：确保使用了 `services/web/nginx.conf` 配置文件。

## 🔐 安全最佳实践

1. ✅ **非 root 用户运行**：所有服务都使用非 root 用户
2. ✅ **最小化镜像**：使用 alpine/slim 基础镜像
3. ✅ **多阶段构建**：减小最终镜像体积
4. ✅ **健康检查**：确保服务可用性
5. ✅ **无敏感信息**：不在镜像中硬编码密钥

## 📊 镜像大小参考

| 服务 | 预估大小 | 说明 |
|------|---------|------|
| API | ~20-30 MB | Go 二进制 + alpine |
| Web | ~50-80 MB | 静态文件 + nginx |
| Bot | ~150-200 MB | Python + 依赖 |

---

**相关文档**：
- [系统架构](../system-architecture.md)
- [开发指南](../reference/development-guide.md)
- [部署指南](./deployment.md)
