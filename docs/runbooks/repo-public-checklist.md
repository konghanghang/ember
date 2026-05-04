# 仓库公开前置 Checklist

> 用途：Ember 仓库从 private 切为 public 时的最后核查清单。
> 维护者：Ember
> 最后更新：2026-05-04

按"无法回退的最先做、可灰度的最后做"排序。每条勾完才进入下一条。

## 1. 敏感信息扫描

仓库公开后所有历史 commit 永久可见，外部一旦 fork，即使重写历史也保不住。先扫一遍再做任何后续动作。

- [ ] `gitleaks detect --source . --no-banner` 扫历史。
- [ ] `git log -p | grep -iE "password|secret|token|api[_-]?key|bearer|BEGIN.*PRIVATE"` 复查 gitleaks 漏报项。
- [ ] CI workflow 内 secret 引用全部走 `${{ secrets.X }}`，无明文。
- [ ] `infrastructure/docker/.env*`、`services/*/.env*` 等本地配置文件无真实 credential。

发现泄露时：先撤销 / 轮换密钥；是否需要 BFG / git-filter-repo 重写历史另行评估（一旦外部已 fork，重写效果有限）。

## 2. GHCR 镜像翻公开

仓库公开但镜像仍 private，OSS 用户首次 `docker pull` 会撞 `denied: not authorized`，是首次部署的硬阻塞。

- [ ] 核实当前可见性：

  ```bash
  for pkg in ember-api ember-web ember-bot; do
    gh api /users/konghanghang/packages/container/$pkg | jq .visibility
  done
  ```

- [ ] 翻公开（private 时执行）：

  ```bash
  for pkg in ember-api ember-web ember-bot; do
    gh api -X PATCH /user/packages/container/$pkg -f visibility=public
  done
  ```

- [ ] 在未登录 GHCR 的环境验证 `docker pull ghcr.io/konghanghang/ember-api:<最近 tag>` 成功。

## 3. README 与项目文件自检

- [ ] 顶层 `README.md` quickstart 不依赖任何外链可独立跑通（参见 `docs/plan/architecture/oss-deployment-experience.md` phase 1 收口结果）。
- [ ] LICENSE 文件存在且明确（默认推荐 MIT 或 Apache-2.0）。
- [ ] `CONTRIBUTING.md` / `CODE_OF_CONDUCT.md` 至少有最小版本（按需）。
- [ ] 所有相对路径 markdown 链接可点开。

## 4. Issue / PR / Discussions 模板

- [ ] `.github/ISSUE_TEMPLATE/` 至少含 `bug_report.yml` 与 `feature_request.yml`。
- [ ] `.github/PULL_REQUEST_TEMPLATE.md` 存在。
- [ ] Settings → Features → Discussions：按需启用并配置 Categories（至少 Q&A、Ideas）。

## 5. 仓库设置

- [ ] Default branch 与 Branch protection（PR review / status check）确认。
- [ ] Settings → Secrets and variables → Actions：核实仅保留必要 secret，无遗留个人 token。

## 6. 执行翻公开

确认 1-5 全部勾完后：

- [ ] Settings → General → Danger Zone → Change repository visibility → Public。
- [ ] 翻公开后立即验证：匿名浏览器访问仓库可见，匿名 `docker pull` 三个镜像均成功。

## 7. 翻公开后

- [ ] 在 Discussions / Issues 发布公告。
- [ ] 1 周内监控 issue / discussion，确保首次部署反馈被及时回应。
