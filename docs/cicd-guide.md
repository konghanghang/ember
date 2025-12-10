# CI/CD 使用指南

## 📋 概述

Ember 使用 GitHub Actions 实现自动化的持续集成和持续部署。

## 🔄 CI - 持续集成

**触发条件**：
- Push 到 `master`、`main` 或 `develop` 分支
- 创建 Pull Request 到 `master` 或 `main` 分支

**验证内容**：
1. ✅ Prisma Schema 验证
2. ✅ 生成 Prisma Client
3. ✅ 代码风格检查（ESLint）
4. ✅ Next.js 编译构建

**查看结果**：
- GitHub 仓库 → Actions 标签页
- Pull Request 页面会显示检查状态

---

## 🚀 Release - 发布流程

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

### Docker 镜像标签

每次发布会生成以下标签：

```bash
# 完整版本号
ghcr.io/<用户名>/ember:v1.2.3
ghcr.io/<用户名>/ember:1.2.3

# 主版本号 + 次版本号
ghcr.io/<用户名>/ember:1.2

# 主版本号
ghcr.io/<用户名>/ember:1

# 最新版本
ghcr.io/<用户名>/ember:latest
```

**使用示例**：

```bash
# 拉取最新版本
docker pull ghcr.io/<用户名>/ember:latest

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
