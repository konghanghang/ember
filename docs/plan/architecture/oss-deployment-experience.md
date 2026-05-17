# OSS 部署体验实现方案

> 状态：Phase 1 + Phase 2 代码与文档同步均已落地，等首次完整发版与 OSS 用户首次部署反馈后归档
> 负责人：Ember
> 更新时间：2026-05-04

## 背景

Ember 已经具备 monorepo + GHCR 多镜像 + Docker Compose 的标准部署链路，但当前链路是**面向"已经懂的部署者"**设计的，对开源用户的首次部署门槛过高：

- 部署者复制 `infrastructure/docker/.env.example` 后执行 `docker compose up`，会被 `${EMBER_API_IMAGE:?}` / `${EMBER_WEB_IMAGE:?}` / `${EMBER_BOT_IMAGE:?}` 三个 compose 端强制变量直接拦下，**而 `.env.example` 中并未列出这三个变量**。这是首次部署的硬阻塞，且报错信息不指向解决路径。
- GHCR workflow 仅构建 `linux/amd64`，把所有 ARM 用户群体（Apple Silicon / 树莓派 / Oracle Cloud ARM / AWS Graviton 等）排除在预构建镜像之外，他们必须本地构建，门槛骤升。
- `.env.example` 中所有密钥都是 `your-secret-...` 占位字符串，没有"如何生成强密钥"的指引（例如 `openssl rand -hex 32`），用户复制 placeholder 起生产环境是真实风险。
- README 顶部的 quickstart 章节并不自洽：第 3 步"按部署指南补齐必填项"是个黑洞，必须跳到 `deployment-environment.md` 才能继续，首次接触跳转 4-5 次链接才能跑起来。
- `DATABASE_URL` 需要部署者手工拼接，并与 `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` 三处保持一致；任意一处不一致就是连不上数据库。
- `ember-bot` 在 compose 中默认启动，但很多 OSS 用户不开 Bot；部署文档建议"手动注释 `ember-bot` 服务"——这是反模式。
- 升级路径目前还需要部署者手工跑 SQL（已有独立的 `database-migration-auto-apply` 方案在推进），文档与实际部署体验之间存在缺口。

如果继续维持现状，OSS 用户的首次接触体验在"项目可见 → 跑起来"这一步会被一连串可避免的小阻塞耗掉耐心，star/fork 转化为实际部署的比例会被人为压低。这些阻塞与代码架构无关，本次方案要把它们一次性收口。

## 目标

本方案要实现两个 phase，phase 1 独立可发，phase 2 依赖 `database-migration-auto-apply` 方案落地后启动：

### Phase 1（OSS 阻塞收口，独立可发）

1. 部署者从 `git clone` 到第一次成功 `docker compose up -d` 之间**不再撞 image tag 强制变量的墙**：`.env.example` 自洽，所有 compose 端 `${X:?}` 变量都有对应条目并给出可直接使用的默认值或清晰的填法。
2. ARM64 服务器与 Apple Silicon 用户**可以直接 `docker compose pull`** 拉到原生镜像，不再被迫本地构建。
3. README 顶部的 quickstart 章节**自洽**：脱离任何外链，按文档命令复制即可跑通 `postgres + ember-api + ember-web` 最小集；首次登录路径写明（访问哪个 URL、如何拿到管理员口令）。
4. `.env.example` 显式给出**密钥生成指引**（如 `openssl rand -hex 32`），并对每个必填密钥的语义、长度、来源做单行注释，不再保留 `your-secret-...` 占位。
5. `DATABASE_URL` 不再需要部署者手工拼接：通过 compose 内部从 `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` 自动组装，消除三处一致性的踩坑面。
6. `ember-bot` 默认**不启动**：通过 compose `profiles: ["bot"]` 控制，需要 Bot 时显式 `--profile bot up -d`；用户不开 Bot 时无需注释 yaml 文件。
7. 镜像构建链路（多架构、tag 策略）就位；GHCR 包翻公开作为仓库从 private 切 public 时的前置项，统一收口到 [`docs/runbooks/repo-public-checklist.md`](../../runbooks/repo-public-checklist.md)，本方案不直接执行翻公开。

### Phase 2（升级体验收口，依赖迁移方案落地）

8. README quickstart 的"升级流程"段落与 `database-migration-auto-apply` 方案的"落地后文档处理"对齐：升级流程精简为 `docker compose pull && docker compose up -d`，不再需要部署者手工 SQL。
9. 与迁移方案的"非目标"段落（`infrastructure/docker/initdb/` 双轨退役）对接，本方案在 phase 2 内同步评估：若迁移方案稳定运行≥1 个发版周期，可在本方案 phase 2 中执行 `initdb/` 退役并简化 compose；否则维持双轨。

## 非目标

本方案明确不做：

- **不合并镜像**：API / Web / Bot 维持三个独立镜像，PostgreSQL 维持官方镜像。把多服务塞进一个镜像是反模式（数据耦合、进程管理代价、镜像体积、更新粒度），已在前置讨论中排除。
- **不引入 setup wizard / 一键安装脚本**：首次启动后的设置中心配置（Emby / TMDB / MoviePilot）维持现状（在控制台补齐），本方案不新增 UI 引导步骤、不新增 `curl ... | bash` 脚本。
- **不改 API / Web / Bot 任何业务代码**：本方案只触达 `.env.example` / `docker-compose.yml` / `.github/workflows/build-*.yml` / 顶层 `README.md` / runbook 文档；不动 Go / Vue / Python 业务实现。
- **不重写部署文档体系**：现行 `deployment.md` / `deployment-environment.md` / `deployment-troubleshooting.md` / `docker-build-guide.md` / `release-process.md` 单一职责拆分合理，本方案只做内容修订与对齐，不做目录重构。
- **不引入 Caddy / Nginx HTTPS 反代默认配置**：HTTPS 链路属于"用户的接入方式选择"，不应在 compose 主体内默认启用；现有注释掉的 Nginx 段落可以维持或迁出到 `examples/` 下作为参考片段，但不在本方案 P0/P1 范围内。
- **不实现镜像签名（cosign）/ SBOM 生成**：属于供应链安全增强，单独立项更合适；当前优先级低于 ARM 用户能拉到镜像。
- **不改 GHCR namespace 与现有 tag 策略**：维持 `ghcr.io/konghanghang/ember-{api,web,bot}` + `:版本号` / `:latest` / `:preview-*` 现行约定，仅追加多架构支持。
- **不在本方案中做 baseline 收口或 schema 改动**：迁移方案自有边界，本方案不越界。

## 当前事实

以当前代码和现行文档为准：

- 相关文档：
  - `README.md`：顶层 quickstart 章节
  - `infrastructure/docker/README.md`：Docker 目录说明
  - `infrastructure/docker/.env.example`：环境变量模板
  - `infrastructure/docker/docker-compose.yml`：compose 主入口
  - `docs/runbooks/deployment.md`：部署最短路径
  - `docs/runbooks/deployment-environment.md`：变量字典与升级流程
  - `docs/runbooks/docker-build-guide.md`：本地构建说明
  - `docs/runbooks/release-process.md`：发布流程
- 相关服务 / 配置：
  - `.github/workflows/build-api.yml` / `build-web.yml` / `build-bot.yml`：镜像构建 workflow
  - `.github/workflows/create-release.yml`：自动 release notes
  - `services/api/Dockerfile` / `services/web/Dockerfile` / `services/bot/Dockerfile`：镜像构建定义
- 当前行为：
  - compose 强制 `EMBER_API_IMAGE` / `EMBER_WEB_IMAGE` / `EMBER_BOT_IMAGE` 三项，但 `.env.example` 不含这三项。
  - workflow 仅 `linux/amd64`，preview 与 release 均如此。
  - `.env.example` 列出所有必填项但密钥为占位字符串，无生成指引。
  - `DATABASE_URL` 在 `.env.example` 内手工拼接，依赖部署者保证三处一致。
  - `ember-bot` 默认启动，关闭需手动注释 yaml。
  - 升级流程已由 `database-migration-auto-apply` 方案接管（v1.4.x 起），部署者执行 `docker compose pull && up -d` 即可；本计划 Phase 2 仅需对齐升级文档表述与评估 `initdb/` 退役。
- 现有限制：
  - GHCR 包当前默认为 private（GitHub 默认行为），核实与翻公开统一收口到 `docs/runbooks/repo-public-checklist.md`，不在本方案 phase 1 范围内。
  - 顶层 `README.md` quickstart 不自洽，依赖跳转 `deployment.md` → `deployment-environment.md` 才能补齐变量。
  - 没有"首次登录路径"指引（用户跑起来后不知道访问哪个 URL、用什么账号登录）。

## 方案设计

### 1. 用户可见行为

#### Phase 1

新增：

- 部署者按顺序执行 `git clone → cd ember → cp infrastructure/docker/.env.example .env → openssl rand -hex 32 ...（按 .env 注释生成密钥）→ docker compose -f infrastructure/docker/docker-compose.yml up -d`，即可拉起 `postgres + ember-api + ember-web` 最小集，**不开 Bot**。
- 用户在仓库根目录 / `infrastructure/docker/` 下都能直接 `docker compose up`，不强制要求 `cd` 到子目录。
- ARM 用户与 Apple Silicon 用户 `docker compose pull` 命中原生 arm64 镜像，无需 QEMU 模拟运行。
- 启用 Bot 时显式 `docker compose --profile bot up -d`，关闭 Bot 时不再需要修改 yaml。

修改：

- `infrastructure/docker/.env.example` 字段集扩展（新增 image 变量、密钥生成提示）；占位密钥改为指引而非明文值。
- `infrastructure/docker/docker-compose.yml`：Bot 服务挂 `profiles: ["bot"]`；`DATABASE_URL` 不再要求外部注入，由 compose 内部从其它 POSTGRES_* 变量拼接（外部仍可覆盖）。
- 顶层 `README.md` 的 quickstart 章节：从 5 步链接跳转改写为自洽的 "命令清单 + 首次登录指引" 段落。
- `.github/workflows/build-api.yml` / `build-web.yml` / `build-bot.yml`：release tag 触发的构建启用 `linux/amd64,linux/arm64` 双架构。

必须保持不变：

- compose 现有 `${X:?}` 强制把关风格（除 image tag 给出合理默认值外，密钥类变量仍强制非空）。
- 镜像 tag 不允许 floating `:latest` 在生产用（compose 端可以兜底默认到一个钉版 tag，但仍必须显式可见）。
- 现有 `deployment.md` / `deployment-environment.md` / `deployment-troubleshooting.md` 的职责拆分。
- `services/api` / `services/web` / `services/bot` 任何 Go / Vue / Python 业务行为。
- GHCR namespace、tag 策略、release 草稿生成行为。
- API 启动期 `InitDB → VerifySchema → Bootstrap` 序列。

#### Phase 2

新增：

- 升级流程在文档中收口为：`docker compose pull && docker compose up -d`，部署者无需手工 SQL（依赖迁移方案的 `ember-migrate` 容器已就位）。

修改：

- `README.md` quickstart 的"升级"段落、`deployment-environment.md` 升级章节、`infrastructure/docker/README.md` 升级提示，均按迁移方案"落地后文档处理"章节同步收口。
- 评估并执行 `infrastructure/docker/initdb/` 双轨退役（条件见 phase 2 关键流程）。

### 2. 数据与模型

> 本次不涉及数据模型变更。

### 3. 接口与边界

#### Phase 1 边界

`.env.example` 字段集变化：

> 决策：image tag 默认值放在 `docker-compose.yml`（见下文 compose 变化），`.env.example` 不列出 image 变量；OSS 用户连 image 字段都不用填。

| 变量 | 原状态 | 新状态 | 语义 |
|---|---|---|---|
| `DATABASE_URL` | 必填，手工拼接 | 可选，缺省由 compose 内部按 `POSTGRES_USER/PASSWORD/DB` 拼接；外部覆盖时仍取外部值 | DB 连接串 |
| `JWT_SECRET` | 占位字符串 | 占位改为生成命令注释，统一推荐 WSL / Git Bash + `openssl rand -hex 32` | JWT 签名密钥 |
| `CONFIG_ENCRYPTION_KEY` | 占位字符串 | 同上 | 设置中心加密主密钥 |
| `INTERNAL_API_SECRET` | 占位字符串 | 同上 | API↔Bot 共享密钥 |
| `TELEGRAM_WEBHOOK_SECRET` | 占位字符串 | 同上 | Bot webhook 验签密钥 |

`docker-compose.yml` 变化：

- `ember-bot` 服务追加 `profiles: ["bot"]`；其他服务无 profile，保持默认启动。
- `ember-api` 与 `ember-bot` 服务的 `DATABASE_URL` / 内部依赖 DB 连接的环境变量改为 compose 内部插值（如 `DATABASE_URL: ${DATABASE_URL:-postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB:-ember}?sslmode=disable}`），允许外部覆盖。
- `EMBER_API_IMAGE` / `EMBER_WEB_IMAGE` / `EMBER_BOT_IMAGE` 在 compose 中给出合理默认值（钉到与当前仓库版本对齐的稳定 tag），将 `${X:?}` 改为 `${X:-默认 tag}`；`.env.example` 不再列出这三项。同时在 compose 顶部注释强调"生产环境推荐显式钉版"，保留显式覆盖路径。

`.github/workflows/*` 变化：

- release tag 触发的构建：`platforms: linux/amd64,linux/arm64`。
- pre_release 分支触发的预览构建：维持 `linux/amd64`（构建时间敏感，预览仅用于内部自查）。

不修改的接口：

- HTTP API、Internal API、Bot webhook、Telegram 命令、cron 入口。
- GHCR namespace、tag 命名策略、release 草稿生成。
- `infrastructure/database/` SQL 真相目录、`initdb/` 同步规则（phase 2 才可能调整）。

#### Phase 2 边界

不引入新接口；仅同步以下文档段落：

- `README.md` quickstart "升级"段落
- `docs/runbooks/deployment.md` 升级章节
- `docs/runbooks/deployment-environment.md` "数据库迁移策略" 章节
- `infrastructure/docker/README.md` `initdb/` 子目录说明（如执行 `initdb/` 退役）

### 4. 关键流程

#### Phase 1 部署者首次部署流程（目标态）

1. `git clone` 仓库，`cd ember`。
2. `cp infrastructure/docker/.env.example .env`。
3. 按 `.env` 注释生成密钥（命令直接复制即可），填入 4 个密钥变量。
4. 决定是否启用 Bot：
   - 不启用：直接 `docker compose -f infrastructure/docker/docker-compose.yml up -d`。
   - 启用：在 `.env` 填 `TELEGRAM_BOT_TOKEN` 等 Bot 变量，执行 `docker compose --profile bot up -d`。
5. compose 拉取镜像（默认 image tag 已钉到稳定版），启动 `postgres → ember-api → ember-web`（→ `ember-bot` 当 profile 启用时）。
6. 浏览器访问 `http://localhost`，按 README 写明的"首次登录指引"完成首次登录（包含临时口令获取方式、改密路径）。

#### Phase 1 GHCR 多架构构建流程

1. release tag (`v*`) 推送 → workflow 触发。
2. `docker/setup-qemu-action` 注入 binfmt → `docker/setup-buildx-action` 创建 multi-arch builder。
3. `docker/build-push-action` 以 `platforms: linux/amd64,linux/arm64` 构建并推送一份 multi-arch manifest。
4. GHCR 上同一 tag 提供两个架构的 image manifest list；`docker pull` 自动按宿主架构选取对应 image。

约束：

- preview 维持 amd64-only（开发自查用，构建速度优先）。
- 三个 workflow 同步改造，避免 API arm64 但 Web 仍 amd64 这类不一致。

#### Phase 2 升级流程（目标态）

1. 部署者执行 `docker compose pull`（拉到新版本镜像，含迁移方案的 `ember-migrate` 二进制与最新 SQL）。
2. 执行 `docker compose up -d`：`ember-migrate` 容器先跑、应用未应用的 SQL、退出 0；`ember-api` 才启动。
3. 部署者无需任何手工 SQL 操作。

#### Phase 2 `initdb/` 退役评估

启动条件（必须**同时满足**）：

1. 迁移方案在主线稳定运行 ≥ 1 个发版周期，未出现 backfill / checksum 误判事故。
2. 新装库通过 `ember-migrate` 直接从空库一次性应用全部 SQL（不再依赖 PG initdb 跑 baseline）这一路径已被验证。

满足后：

- 删除 `infrastructure/docker/initdb/` 目录与 `docker-compose.yml` 中对应挂载行。
- 删除 `infrastructure/database/README.md` 与 `infrastructure/docker/README.md` 中的"双源同步"段落。
- 单写一个小型收口子计划（不开新 plan），在本方案归档前完成。

### 5. 失败路径与边界条件

#### Phase 1 失败路径

- **arm64 构建失败**：单个 workflow 任意架构失败则整个 push 失败，避免出现 GHCR 上"amd64 manifest 已推、arm64 manifest 缺失"的不一致状态。回滚策略：临时把 `platforms` 退回到 `linux/amd64`，再单独定位 arm64 失败原因。
- **image tag 默认值漂移**：compose 给定的默认 image tag 必须随每次发版同步更新，否则新部署用户拉到的是过时镜像。约束：将"更新 compose 默认 image tag"列入 `release-process.md` 的发版 checklist；CI 可加一道校验"compose 默认 tag 与最新 release tag 是否一致"（可选增强，不在 P0）。
- **Bot profile 默认关闭后老部署回归测试**：现有线上部署若使用旧版 compose（无 profile），升级到新版后需要显式 `--profile bot up -d` 才会启动 Bot。约束：在 `release-process.md` / 升级 release notes 中显式提醒，并给出"如何检查 ember-bot 是否在运行"的命令。
- **`DATABASE_URL` 自动拼接与现有覆盖逻辑冲突**：现有 `.env.example` 显式提供 `DATABASE_URL`，部分用户可能依赖它指向独立 DB（非 compose 内的 postgres）。约束：compose 内部拼接仅在 `DATABASE_URL` 缺省时生效，外部覆盖路径必须保留并在 `.env.example` 注释里写清。
- **密钥生成指引在 Windows 用户场景下不适用**：`openssl rand -hex 32` 在 Windows 原生命令行不可用。约束：注释统一推荐 WSL / Git Bash + `openssl rand -hex 32`，不再维护 PowerShell 等价命令。

兼容性约束：

- 不能破坏现有部署者的工作流：已显式覆盖 `EMBER_*_IMAGE` / `DATABASE_URL` 的部署不能因为 compose 内部拼接而行为变化。
- 不能让 amd64-only 用户因 multi-arch 改动而拉到错误架构镜像（manifest list 自动选架构是 Docker 标准行为，正常应无影响，但需在 release notes 中说明）。
- 不能改写 `.env.example` 中已有的、目前正确的注释（仅做增量与占位字符串替换）。

回滚策略：

- Phase 1 任意子项上线后出现严重问题，可以单独回滚该子项（每条改动独立提交）。
  - image tag 默认值改坏：用户可在 `.env` 显式覆盖恢复行为。
  - Bot profile：用户可在 `.env` 设置 `COMPOSE_PROFILES=bot` 或显式 `--profile bot`。
  - arm64 构建：workflow 退回 amd64-only。
  - `DATABASE_URL` 自动拼接：用户在 `.env` 显式提供 `DATABASE_URL` 即可绕过。

#### Phase 2 失败路径

- **迁移方案 ember-migrate 上线后出现回归**：phase 2 不能启动；现状文档维持手工 SQL 升级路径不变。
- **`initdb/` 退役评估未通过**：维持双轨现状，本方案 phase 2 仅完成升级文档收口，不强行退役。

## 影响范围

涉及的子系统：

- API：无。
- Web：无。
- Bot：仅 compose profile 控制启停，业务代码无改动。
- 配置 / 部署：
  - `infrastructure/docker/.env.example`：字段集扩展、占位字符串改为生成指引
  - `infrastructure/docker/docker-compose.yml`：Bot profile、DATABASE_URL 内部拼接、image tag 默认值
  - `.github/workflows/build-api.yml` / `build-web.yml` / `build-bot.yml`：release 触发的多架构构建
  - 仅保留 `infrastructure/docker/.env.example` 单源；README quickstart 用 `cp infrastructure/docker/.env.example .env`
- 文档：
  - `README.md`：顶层 quickstart 自洽化、首次登录指引、ARM 用户说明
  - `infrastructure/docker/README.md`：profile 用法、image tag 默认值说明
  - `docs/runbooks/deployment.md`：phase 2 升级流程同步
  - `docs/runbooks/deployment-environment.md`：phase 2 升级章节同步、DATABASE_URL 自动拼接说明
  - `docs/runbooks/release-process.md`：发版 checklist 追加"更新 compose 默认 image tag"
  - `docs/runbooks/repo-public-checklist.md`：新增仓库公开前置 checklist，承接 GHCR 翻公开等动作（与本方案解耦，仓库公开当天执行）
  - `docs/system-architecture.md`：如 phase 2 执行 initdb/ 退役，同步数据库章节

前端约束：

- 本方案不涉及前端页面、组件、交互或视觉改动；不需要遵循 `docs/reference/web-design-guide.md`。

## 验证方式

### 编译 / 测试

- `cd infrastructure/docker && docker compose config`（compose 文件语法与变量插值校验）
- `cd infrastructure/docker && docker compose --profile bot config`（profile 启用路径校验）
- 在 amd64 与 arm64 环境分别执行 `docker compose pull && docker compose up -d`（拉取多架构 manifest 并启动）
- 顶层 `cd services/api && go build ./... && go test ./...`（无业务改动，仅作 sanity check）
- 顶层 `cd services/web && npm run build`（同上）

### 手工验证

#### Phase 1

- **新克隆 + 默认部署**：clean 环境 `git clone` → 按 README quickstart 复制命令 → 拉起最小集（不开 Bot）→ 浏览器访问 `http://localhost` 看到登录页 → 按指引完成首次登录。
- **启用 Bot 部署**：在 `.env` 填 Bot 变量 → `docker compose --profile bot up -d` → `ember-bot` 容器启动，`docker compose ps` 可见。
- **不启用 Bot 部署**：不填 Bot 变量、不加 profile → `ember-bot` 不启动，其他三个服务正常。
- **ARM 服务器拉镜像**：在 Apple Silicon / arm64 服务器上 `docker compose pull` → 拉到 arm64 manifest，`docker inspect <image> | grep Architecture` 输出 `arm64`。
- **DATABASE_URL 覆盖路径**：在 `.env` 显式提供独立 DB 的 `DATABASE_URL` → API 连接到该独立 DB，不走 compose 内部 postgres。
- **DATABASE_URL 缺省路径**：`.env` 不提供 `DATABASE_URL` → API 连接到 compose 内部 postgres，连接成功。
- **密钥生成指引可执行**：按 `.env.example` 注释命令执行 `openssl rand -hex 32` 输出可直接填入。

#### Phase 2

- **升级流程**：使用 phase 1 部署的环境，按迁移方案落地后的镜像执行 `docker compose pull && docker compose up -d`，无需手工 SQL，API 正常启动。
- **`initdb/` 退役（如执行）**：清空数据卷 → `up -d` → `ember-migrate` 从空库一次性 backfill 全部 SQL → API 正常启动；与未退役场景行为一致。

## Phase 1 落地记录

Phase 1 全部 7 个子目标已代码落地，待真实发版与公开验证：

| Phase 1 目标 | 落地状态 |
|---|---|
| 1. `.env.example` 自洽（消除 image 强制变量、单源） | ✅ commit `045c032`：image 默认值放在 compose；`.env.example` 不再列 image 变量 |
| 2. ARM64 镜像（release tag 触发双架构） | ✅ commit `bc2ce72`：preview 维持 amd64；ARM64 真实可拉性需下次 release tag push 触发 CI 实测 |
| 3. README quickstart 自洽 | ✅ 顶层 README 改写为 5 步自洽 quickstart（含密钥生成、首次登录指引） |
| 4. `.env.example` 密钥生成指引 | ✅ 4 个 secret + `POSTGRES_PASSWORD` + `ADMIN_PASSWORD` 改为空值 + 内联 `openssl rand -hex N` 命令；header 推荐 WSL / Git Bash |
| 5. `DATABASE_URL` 自动拼接 | ✅ commit `045c032`：缺省由 compose 按 `POSTGRES_USER/PASSWORD/DB` 拼接到内置 postgres，外部覆盖路径保留 |
| 6. Bot profile 默认关闭 | ✅ commit `045c032`：`profiles: ["bot"]`，启用需 `docker compose --profile bot up -d` |
| 7. GHCR 公开性收口 | ✅ commit `06ef582`：翻公开动作挂到 [`docs/runbooks/repo-public-checklist.md`](../../runbooks/repo-public-checklist.md)，仓库公开当天执行 |

待 real-world 验证：

- 下次 `v*` Tag 触发 release workflow，验证 ARM64 三个镜像构建成功（Bot 的 Python wheel 依赖最易踩）
- 仓库公开当天按 `docs/runbooks/repo-public-checklist.md` 执行 GHCR 翻公开
- OSS 用户首次部署反馈（按 README quickstart 跑通最小集）

## Phase 2 落地记录

Phase 2 全部 2 个子目标已代码落地，等首次完整发版验证：

| Phase 2 目标 | 落地状态 |
|---|---|
| 8. README quickstart 升级流程对齐自动迁移 | ✅ README quickstart 末尾追加"升级到新版本"小节（`docker compose pull && up -d`，迁移自动）；`docs/runbooks/deployment.md` / `deployment-environment.md` / `infrastructure/docker/README.md` / `infrastructure/database/README.md` / `docs/system-architecture.md` 同步收口 |
| 9. `initdb/` 退役评估 | ✅ 直接执行退役：删除 `infrastructure/docker/initdb/` 目录与 compose 中 `./initdb:/docker-entrypoint-initdb.d` 挂载；schema 初始化与升级全部由 `ember-api` 启动期 Migrate 阶段接管，新空库走"新空库"分支按字典序 forward-only 应用全部 SQL |

退役决策依据（与原方案"启动条件"的偏离）：

- 原方案 phase 2 目标 9 启动条件要求"迁移方案稳定运行 ≥ 1 个发版周期"；迁移方案自身的退役条件更严（≥ 3 次含新增 fingerprint 发版 + ≥ 1 个月稳定期 + 单独立计划）
- 实际取舍：当前 OSS 项目**尚未正式发版给用户**（无活跃 OSS 部署），删除 `initdb/` 挂载对存量数据卷无影响（PG initdb 本来也不会再触发），新空库分支已被 `migrate_test.go` 自动化测试覆盖
- 收益：双轨简化为单轨，消除"双源同步"维护面，OSS 用户首次部署链路更简单

代码 / 文档配合改动：

- `services/api/internal/db/migrate.go`：新空库分支启动日志增强为"新空库（未检测到既存业务表）→ 将从空库一次性初始化 schema"，便于 OSS 用户首次部署时辨识；`migrateBranchMixed` 注释改写为"业务核心表已存在但 fingerprint 不齐"的边界场景
- `services/api/internal/db/migrate_test.go`：混合模式测试用例的注释同步改写
- `docs/archive/plan/architecture/database-migration-auto-apply.md`：顶部追加演进说明 + 用户可见行为段、混合模式分支动机段、验证用例段同步收口

## 落地后文档处理

### Phase 1 落地后

- 同步更新 `README.md` quickstart、`infrastructure/docker/README.md`、`docs/runbooks/deployment.md`、`docs/runbooks/deployment-environment.md`、`docs/runbooks/release-process.md`。
- 在 release notes 中显式说明：
  - Bot 默认行为变化（profile）
  - image tag 默认值机制
  - ARM64 镜像可用
- 仓库根 README 顶部 "Docker Compose 部署模式" 段落改写为自洽 quickstart。

### Phase 2 落地后

- ✅ 与迁移方案的 "落地后文档处理" 章节交叉确认：升级流程在所有文档中的描述一致。
- ✅ 已执行 `initdb/` 退役，同步 `docs/system-architecture.md` 数据库章节、`infrastructure/database/README.md`、`infrastructure/docker/README.md`。
- 首次发版后在 release notes 中说明：
  - PG `initdb.d` 已退役，新空库由启动期 Migrate 接管
  - 升级路径仍为 `docker compose pull && up -d`（与现行流程一致）

### 归档条件

本计划迁入 `docs/archive/plan/architecture/` 的条件：

1. Phase 1 全部子项已上线且至少经过一个 OSS 用户首次部署正反馈周期（可由 issue / discussion 反馈印证）。
2. ✅ Phase 2 已与迁移方案完成对齐（升级文档收口完成）；`initdb/` 退役已执行。
3. 顶层 `README.md` quickstart 自洽性经过至少一次"clean 环境跟读跑通"验证。

满足以上条件后，本计划迁入 `docs/archive/plan/architecture/`。
