---
name: ember-release
description: 准备、校验、提交并执行 Ember 稳定版本发布。用于在 Ember 仓库中准备新版本、执行上线前检查、更新 Release Notes 或 Docker 镜像 Tag、创建和推送签名版本 Tag，以及检查 GitHub Actions 和 Draft Release。
---

# Ember 版本发布

## 事实来源与执行边界

1. 修改任何内容前，先读取仓库根目录的 `AGENTS.md` 和 `docs/runbooks/release-process.md`，并以发布 runbook 作为流程事实来源。
2. 只在 Ember 仓库根目录执行发布流程，并使用用户明确指定的目标版本。版本号会影响文件或 Git ref 时，禁止自行猜测。
3. 发布验证期间禁止启动项目服务，禁止真实调用 Emby、TMDB、MoviePilot、Stripe 或 Telegram。
4. 禁止自动发布 GitHub Draft Release，禁止操作生产环境。最终公开 Release 和生产部署必须由用户手工完成。
5. 热修复发布中禁止擅自执行 `npm audit fix`、依赖升级、数据库变更或无关清理。发现额外风险时单独报告。
6. 严格保留确认门禁：准备发布材料不代表允许提交；允许提交不代表允许推送；发布提交已推送也不代表允许创建或推送 Tag。

## 发布流程

### 1. 确认发布基线

1. 确认当前分支、工作区、本地 `HEAD` 和 `origin/master` 状态。
2. 只有在需要保证对比基线最新时，才拉取远端 ref 和 Tag。
3. 确认目标版本符合 `vX.Y.Z`、高于上一稳定版本，并且本地和远端都不存在同名 Tag。
4. 执行 `.agents/skills/ember-release/scripts/release_preflight.sh <version> prepare`。
5. 使用 `git log --oneline <previous-tag>..HEAD` 和 `git diff --name-status <previous-tag>..HEAD` 检查真实发布范围。
6. 从实际 diff 中识别用户可见变化、配置变化、migration、部署变化和兼容边界，禁止只根据 commit subject 编写发布说明。

出现以下任一情况时立即停止并报告：当前分支不是 `master`、工作区包含无关改动、本地与远端 `master` 不一致，或目标 Tag 已存在。

### 2. 准备发布材料

只更新发布 runbook 要求的版本入口：

1. 更新 `infrastructure/docker/docker-compose.yml` 中三个默认镜像 Tag：
   - `EMBER_API_IMAGE`
   - `EMBER_WEB_IMAGE`
   - `EMBER_BOT_IMAGE`
2. 以 `docs/releases/release-template.md` 为结构基线，新增 `docs/releases/<version>.md`。
3. 更新 `docs/releases/README.md` 中的稳定版本示例。
4. 确保 Release Notes 与 `<previous-tag>..HEAD` 的实际变化完全一致。
5. 明确说明是否包含 migration、是否需要手工执行 SQL、是否修改配置，以及最低直接升级支持版本是否变化。
6. Compare 链接统一使用 `https://github.com/konghanghang/ember/compare/<previous-tag>...<version>`。

禁止宣称未经验证的兼容性。部署环境或外部系统行为证据不足时，必须明确标记“未验证”或“未证实”。

### 3. 执行发布验证

先执行：

```bash
.agents/skills/ember-release/scripts/release_preflight.sh <version> materials
```

然后执行以下项目检查。

API：

```bash
cd services/api
go test ./...
go vet ./...
go build ./...
```

Web：

```bash
cd services/web
npm ci
npm run test
npm run build
```

如果全局 npm cache 存在所有权问题，改用 `npm ci --cache /tmp/ember-npm-cache`，禁止使用 `sudo`，也不要修改全局目录所有权。必要时可以运行 `npm audit --omit=dev` 作为风险检查，但禁止在发布提交中自动修复。

Bot：

```bash
cd services/bot
.venv/bin/pip install -r requirements.txt
.venv/bin/python -m py_compile main.py
.venv/bin/python -m pytest tests
```

必须使用仓库内的 `.venv`，禁止回退到系统 Python。使用占位配置通过 `docker compose ... config --quiet` 校验 Compose，禁止执行 `up`、`start` 或启动任何服务进程。

检查 `master` 和 `pre_release` 的最新 GitHub Actions。如果任务持续处于 queued，且没有 runner 和执行步骤，先检查 job 数据和 GitHub Status，禁止在没有证据时修改 workflow YAML。

汇总以下内容后询问用户是否提交：

- 发布材料文件
- Compare 范围
- 本地验证结果
- GitHub Actions 状态
- migration 和配置影响
- 未解决风险

### 4. 提交发布材料

只有用户明确同意提交后才能执行：

1. 只暂存发布材料相关文件。
2. 执行 `git diff --cached --name-status` 和 `git diff --cached --check`。
3. 默认使用提交标题 `docs(release): 准备 <version> 发布材料`；仓库历史存在更明确规范时按现有规范执行。
4. 提交后检查工作区并报告 commit ID。

除非用户明确要求，否则禁止推送提交。

### 5. 创建并推送版本 Tag

同时满足以下条件后才能进入本阶段：

- 发布材料提交已经存在于 `origin/master`
- 用户明确要求创建 Tag

执行步骤：

1. 运行 `.agents/skills/ember-release/scripts/release_preflight.sh <version> tag`。
2. 使用 `git cat-file` 检查上一版本 Tag 的类型和签名方式。
3. 创建相同类型的 Tag。Ember 稳定版本使用带签名的 annotated tag：

```bash
git tag -s <version> -m "<version>"
```

4. 检查新 Tag 对象，确认其指向预期的发布提交。
5. 用户明确授权推送后，只推送目标 Tag：

```bash
git push origin <version>
```

6. 使用 `git ls-remote --tags` 检查远端 Tag 对象和解引用后的 commit。

禁止使用 `git push --tags`。

### 6. 监控发布自动化

Tag 推送后，持续监控以下 GitHub Actions，直到全部结束：

- Build and Push API Image
- Build and Push Web Image
- Build and Push Bot Image
- Create Release

确认 API、Web、Bot 三个正式镜像工作流全部成功，并确认 GitHub Release 以 Draft 状态创建，Tag 和名称均正确。出现失败时，报告 run ID 和失败步骤，禁止自动发布 Draft。

## 失败处理

- Release Notes 缺失、Compare 链接错误、Tag 冲突、工作区不干净、本地与远端分支不一致时，阻止继续发布。
- 编译、测试、Compose 解析、镜像构建或 Draft Release 创建失败时，阻止继续发布。
- 依赖审计问题默认与本次发布 diff 分开处理，除非用户明确扩大任务范围。
- 如果因为未配置 `gpg.ssh.allowedSignersFile` 导致 SSH 签名信任验证失败，检查 commit 或 Tag 对象中的 `gpgsig` 或 SSH signature，并准确说明“签名存在但本机无法完成信任验证”。
- GitHub Actions 处于 queued 时，先确认是否已经分配 runner，禁止仅凭排队状态修改 CI 配置。

## 只读预检脚本

使用 `scripts/release_preflight.sh` 执行确定性的仓库状态检查：

```bash
.agents/skills/ember-release/scripts/release_preflight.sh v1.6.5 prepare
.agents/skills/ember-release/scripts/release_preflight.sh v1.6.5 materials
.agents/skills/ember-release/scripts/release_preflight.sh v1.6.5 tag
```

该脚本只读取仓库和远端 Tag 状态，不修改文件、不创建 commit、不创建 Tag，也不推送任何 Git ref。
