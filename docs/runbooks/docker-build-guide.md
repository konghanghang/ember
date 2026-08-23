# Docker 构建指南

这份文档只讲镜像怎么构建，不负责讲版本发布、Tag 流程或 Release 草稿。

## 镜像划分

Ember 当前发布三个镜像：

| 服务 | 镜像名 | Dockerfile |
|------|--------|------------|
| API / Gateway | `ghcr.io/konghanghang/ember-api` | `services/api/Dockerfile` |
| Web | `ghcr.io/konghanghang/ember-web` | `services/web/Dockerfile` |
| Bot | `ghcr.io/konghanghang/ember-bot` | `services/bot/Dockerfile` |

### Gateway 镜像合同

- 不新增 `ember-gateway` 镜像、独立 GHCR 仓库、构建工作流或版本号。
- `ember-api` 镜像只构建一个 `ember` 二进制，以 `ENTRYPOINT ["./ember"]` 和默认 `CMD ["api"]` 启动 API。
- Compose 中 `ember-gateway` profile 复用完全相同的 `EMBER_API_IMAGE`，只用 `command: ["gateway"]` 选择 Gateway 进程角色。
- 单二进制不表示单进程：API 与 Gateway 仍是两个容器、两个生命周期和两个健康检查，只共享镜像 digest 与代码版本。

当前 Dockerfile 只从 `cmd/ember` 构建统一服务二进制；API 与 Gateway 不再维护其他 main package。

## 本地构建

### API

```bash
# 在仓库根执行，build context 必须是仓库根（Dockerfile 需要 COPY infrastructure/database/）
docker build -f services/api/Dockerfile -t ember-api:dev .
```

镜像验收必须确认容器内只有一个生产 `ember` 二进制，并检查默认 `api` 与 Compose `gateway` command；不能通过构建两个可执行文件伪装成单二进制方案。

### Web

```bash
cd services/web
docker build -t ember-web:dev .
```

如果要让前端控制台侧边栏展示当前镜像对应的 commit hash，本地 Docker 构建时显式传入 build args：

```bash
cd services/web
docker build \
  --build-arg VITE_GIT_COMMIT_SHA="$(git rev-parse HEAD)" \
  --build-arg VITE_GITHUB_REPOSITORY="konghanghang/ember" \
  -t ember-web:dev .
```

### Bot

```bash
cd services/bot
docker build -t ember-bot:dev .
```

## 用 Compose 做本地构建

默认 compose 使用预构建镜像。若要改成本地构建：

1. 打开 [`infrastructure/docker/docker-compose.yml`](../../infrastructure/docker/docker-compose.yml)
2. 保留 `ember-api.image` 作为本地 Tag，取消 `ember-api.build` 注释
3. 不给 `ember-gateway` 添加第二份 build；它直接复用同名本地镜像
4. 执行：

```bash
cd infrastructure/docker
docker compose build
docker compose --profile gateway up -d
```

## GitHub Actions 中的构建流程

当前仓库的镜像构建工作流：

- `.github/workflows/build-api.yml`
- `.github/workflows/build-web.yml`
- `.github/workflows/build-bot.yml`

Gateway 继续由 `build-api.yml` 产出的同一个 `ember-api` 镜像承载，不新增第四个 workflow。

它们的共同规则：

- `pre_release` 分支：生成预览镜像（`linux/amd64`，构建时间敏感）
- `v*` Tag：生成正式镜像（`linux/amd64` + `linux/arm64` 双架构 manifest list，由 `docker/setup-qemu-action` 提供 arm64 binfmt）
- Web 镜像构建会注入 `VITE_GIT_COMMIT_SHA=${{ github.sha }}` 和 `VITE_GITHUB_REPOSITORY=${{ github.repository }}`，用于前端展示源码入口与控制台构建 hash

## 产出的常见标签

### 预览镜像

- `preview`
- `preview-<short-sha>`

### 正式镜像

- `v1.2.3`
- `latest`

发布动作和分支/Tag 规范见 [发布流程](./release-process.md)。

## 本地构建前建议

先跑对应服务的最低验证，别让 Docker 帮你兜编译错误：

### API

```bash
cd services/api
go vet ./...
go build ./...
```

### Web

```bash
cd services/web
npm ci
npm run build
```

### Bot

```bash
cd services/bot
pip install -r requirements.txt
python -m py_compile main.py
```

## 常见问题

### 构建找不到文件

先检查构建上下文是不是对应服务目录，而不是仓库根目录。

### Web 构建后前端路由 404

先检查 `services/web` 镜像是否包含正确的 nginx 配置，而不是先怪前端代码。

### 镜像已更新但服务没变

通常不是镜像没构建，而是部署环境还没拉新镜像并重启容器。

## 相关文档

- [发布流程](./release-process.md)
- [部署指南](./deployment.md)
- [Docker 目录说明](../../infrastructure/docker/README.md)
