# 发布流程

这份文档只讲分支、Tag、CI/CD 和 GitHub Release，不重复解释镜像怎么构建。

## 先分清三件事

- `test.yml`：编译与语法验证
- `build-*.yml`：构建并推送 API / Web / Bot 镜像
- `create-release.yml`：创建 GitHub Draft Release

## 本地预检

发布前至少跑一遍：

```bash
cd services/api && go vet ./... && go build ./...
cd services/web && npm ci && npm run build
cd services/bot && pip install -r requirements.txt && python -m py_compile main.py
```

如果这一步都不过，就别急着打 Tag。

## 发版前 checklist

打 Tag 前必须勾完：

- [ ] 更新 `infrastructure/docker/docker-compose.yml` 中 `EMBER_API_IMAGE` / `EMBER_WEB_IMAGE` / `EMBER_BOT_IMAGE` 的默认 tag 为新版本（compose 默认值是 OSS 用户首次部署的 fallback，必须随发版同步）
- [ ] 确认 `ember-gateway` 与 `ember-api` 引用同一 `EMBER_API_IMAGE`，镜像内只有一个支持 `api/gateway` 子命令的 `ember` 生产二进制；禁止新增独立 Gateway Tag 或漏升其中一个容器
- [ ] 新增 `docs/releases/<version>.md`，文件名与即将推送的 tag 完全一致，并人工核对发布说明
- [ ] 顶层迁移 SQL 已在 `infrastructure/database/` 落地，并确认会随 `services/api/Dockerfile` 打进 API 镜像（PG `initdb.d` 已退役，schema 初始化与升级统一由 `ember-api` 启动期 Migrate 接管）
- [ ] 如本次生成新 baseline 或归档 migration，已明确最低直接升级支持版本，并同步 `infrastructure/database/README.md`、部署 runbook 与 `docs/releases/<version>.md`
- [ ] 本地预检全部通过
- [ ] 发版后部署环境检查 `docker logs ember-api --tail` 中 `[Migrate]` 阶段日志：分支符合预期（新空库、支持窗口内 forward-only、或已声明的 backfill / mixed 场景），无 fail-fast 错误

## CI 触发规则

### 编译验证

`.github/workflows/test.yml` 会在以下场景触发：

- push 到 `master`、`main`、`develop`
- PR 指向 `master`、`main`、`develop`

但它会忽略：

- `docs/**`
- `*.md`
- `LICENSE`
- `.gitignore`

所以文档-only 提交不会自动跑这套 CI。

### 镜像构建

`build-api.yml`、`build-web.yml`、`build-bot.yml` 的触发条件一致：

- push 到 `pre_release`：构建预览镜像
- push `v*` Tag：构建正式镜像

Gateway 由 `build-api.yml` 的同一个 `ember-api` 镜像承载，不新增第四个 workflow。

## 预览发布

适合先让测试环境验证镜像。

```bash
git checkout pre_release
git merge master
git push origin pre_release
```

预期结果：

- API / Web / Bot 都会生成 `preview` 和 `preview-<sha>` 镜像
- 仅用于测试，不建议生产直接吃 `preview`

## 正式发布

```bash
git checkout master
git pull
git tag v1.0.0
git push origin v1.0.0
```

预期结果：

- 三个镜像工作流推送正式镜像
- `ember-api` 镜像同时供 `ember-api` 与 `ember-gateway` 两个容器使用，二者版本必须一致
- `create-release.yml` 读取 `docs/releases/v1.0.0.md` 创建一个 Draft Release
- 如果对应发布说明文件不存在，`create-release.yml` 会 fail-fast，不会创建草稿

## 发布后要做什么

1. 打开 GitHub Releases
2. 找到刚创建的 Draft Release
3. 最后核对 Release Notes，尤其是升级说明
4. 检查镜像标签和升级说明
5. 手动点击发布

如果你不发布 Draft，它就只是个草稿，不是正式对外版本。

## 发布说明怎么准备

Release Notes 是对外发布材料，不再由 workflow 根据 commit 自动推断。每个版本必须在打 tag 前提交同名发布说明文件：

```text
docs/releases/<version>.md
```

例如发布 `v1.6.2` 时，必须先提交：

```text
docs/releases/v1.6.2.md
```

新文件从 [`docs/releases/release-template.md`](../releases/release-template.md) 复制，人工整理本次真正影响用户、部署者或维护者的内容。测试补充、内部重排、计划归档等内容通常不写进对外重点功能。

发布说明至少要人工核对：

- 用户可见功能和修复是否准确
- 数据库迁移、baseline 压缩和最低直接升级支持版本是否写清楚
- 配置项、环境变量、Docker Compose、Bot 启动模式等部署边界是否写清楚
- Docker 镜像 tag 是否与发布 tag 一致
- Compare / Full Changelog 链接是否指向正确版本范围

`create-release.yml` 只负责读取这个文件并创建 Draft Release。找不到文件时会直接失败，避免生成内容看似完整但语义不准的草稿。

## 推荐节奏

- 日常验证：`pre_release`
- 稳定发布：语义化 Tag，如 `v1.2.3`
- 生产部署：固定版本号，不要长期跟 `latest`

## 相关文档

- [Docker 构建指南](./docker-build-guide.md)
- [部署指南](./deployment.md)
- [GitHub Release 模板](../../.github/RELEASE_TEMPLATE.md)
