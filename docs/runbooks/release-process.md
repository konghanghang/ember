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
- `create-release.yml` 创建一个 Draft Release

## 发布后要做什么

1. 打开 GitHub Releases
2. 找到刚创建的 Draft Release
3. 补充 Release Notes
4. 检查镜像标签和升级说明
5. 手动点击发布

如果你不发布 Draft，它就只是个草稿，不是正式对外版本。

## 推荐节奏

- 日常验证：`pre_release`
- 稳定发布：语义化 Tag，如 `v1.2.3`
- 生产部署：固定版本号，不要长期跟 `latest`

## 相关文档

- [Docker 构建指南](./docker-build-guide.md)
- [部署指南](./deployment.md)
- [GitHub Release 模板](../../.github/RELEASE_TEMPLATE.md)
