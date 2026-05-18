# Git 协作规范

本文档记录 Ember 仓库长期有效的 Git 分支、PR 与发布分支约束。完整变更背景写在 PR 中，分支名只负责表达改动类型和主题。

## 分支类型

- `feat/<topic>`：新功能
- `fix/<topic>`：bug 修复
- `refactor/<topic>`：不改变外部行为的重构
- `docs/<topic>`：文档
- `test/<topic>`：测试
- `chore/<topic>`：工程杂项、依赖、配置
- `ci/<topic>`：CI/CD
- `hotfix/<topic>`：线上紧急修复

## 命名规则

- 使用小写英文。
- 单词使用 `-` 连接。
- 不使用中文、空格、下划线。
- 不使用无意义名称，例如 `test`、`fix`、`my-branch`。
- 分支名只表达主题，不承载完整背景。
- 分支名默认使用 `<type>/<short-topic>`，例如 `fix/user-expiry-status`。

## 示例

- `feat/telegram-subscription-approval`
- `fix/user-expiry-status`
- `docs/pr-template`
- `refactor/api-subscription-service`
- `ci/build-api-image`
- `hotfix/payment-callback`

## 需求编号

有 TAPD / Issue 编号时，可以把编号放进主题前缀：

- `feat/tapd-123456-user-invite`
- `fix/issue-42-login-expiry`

没有编号时不要硬凑编号，直接使用清晰的短主题。

## 保护分支约束

- `master` 只接收 PR 合并，不直接推送日常改动。
- 日常开发从 `master` 拉新分支。
- `pre_release` 只用于预览镜像发布。
- 正式发布通过 `v*` Tag 触发。

