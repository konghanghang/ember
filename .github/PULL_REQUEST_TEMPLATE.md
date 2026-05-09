## 变更概述

<!-- 简述本 PR 主要做了什么；不写背景 / 评价 / 过程。 -->

## 关联 Issue

<!-- 例如 Closes #123 / Refs #456；如无可写"无"。 -->

## 改动类型

<!-- 多选请保留多个。 -->
- [ ] feat：新功能
- [ ] fix：bug 修复
- [ ] refactor：重构（不改变外部行为）
- [ ] perf：性能优化
- [ ] docs：仅文档
- [ ] test：仅测试
- [ ] chore / ci / style：工程或风格

## 受影响模块

- [ ] services/api (Go)
- [ ] services/web (Vue)
- [ ] services/bot (Python)
- [ ] infrastructure/database (Schema / Migration)
- [ ] 部署 / Docker Compose / GitHub Actions
- [ ] 文档 (docs/)

## 验证方式

<!-- 至少说明你做了什么验证；纯文档可只做一致性检查。 -->
- [ ] `go build ./...` / `go test ./...` 通过
- [ ] `npm run build` 通过
- [ ] 手测关键路径（请简述测了什么）
- [ ] 文档 / 链接 / 路径一致性检查（仅文档改动）

## 数据库 Schema

<!-- 二选一勾选；同时勾选或都不勾选视为模板未填写。 -->
- [ ] 本 PR 不涉及 schema 变更
- [ ] 涉及 schema 变更，已在 `infrastructure/database/` 提供幂等 SQL migration（命名 `YYYYMMDD_NN_<description>.sql`）

## 文档同步

<!-- 第一条与后面互斥；后面三条可多勾。 -->
- [ ] 本 PR 不需要同步任何文档
- [ ] 已同步 `docs/system-architecture.md`
- [ ] 已同步 `docs/reference/web-design-guide.md`
- [ ] 已同步其他相关文档（请在描述里列出）

## 提交拆分

<!--
一个提交只表达一个主功能 / 一个主修复 / 一个主重构。多个独立功能点应拆成多个 commit，
每个都能被独立 review、独立回滚、独立 cherry-pick。无关改动（格式化、重命名、文档归档）单独提交。
首次贡献不熟悉这条要求时，可在 PR 评论里说明，maintainer 会协助拆分。
-->
- [ ] 本 PR 的每个提交都遵守上述拆分原则

## 其他说明

<!-- 截图、风险点、回滚预案、后续 TODO 等。 -->
