# MoviePilot X-API-KEY 直连改造方案

> 状态：已归档
> 负责人：Ember
> 更新时间：2026-04-13

## 归档说明

本方案已完成落地，当前只保留历史追溯价值。

稳定结论已同步到：

- `docs/system-architecture.md`
- `docs/reference/configuration-reference.md`
- `docs/runbooks/deployment-environment.md`

当前代码已经具备：

- `MoviePilotClient` 使用 `X-API-KEY` 直接访问 `GET /api/v1/site/` 与 `POST /api/v1/subscribe/`
- 设置中心只暴露 `MOVIEPILOT_URL` 与 `MOVIEPILOT_API_KEY`
- 旧版用户名/密码配置只作为迁移提示来源，不再参与实际鉴权
- 订阅审批保持“下游失败不回滚审批状态，错误写入 mpError”的原有语义

因此这份文档不再承担现行实现说明职责。

## 背景

这个问题为什么现在要解决：

- Ember 当前在审批订阅时，先用 `MOVIEPILOT_USERNAME` / `MOVIEPILOT_PASSWORD` 登录 MoviePilot，再拿 `access_token` 调 `POST /api/v1/subscribe/`，链路比实际需要更重。
- 已核实 MoviePilot 自身支持用 `X-API-KEY` 直接访问受保护接口，现有“登录换 Bearer”并不是唯一可行路径。
- 现有配置中心、部署文档和 Docker 环境变量都围绕“用户名密码登录”设计，和目标接法不一致，后续维护成本高。

## 目标

本方案要实现：

1. Ember 改为使用 `X-API-KEY` 直连 MoviePilot，不再依赖登录换取 Bearer Token。
2. 保持现有订阅审批、Bot 通知、`mpError` 回写和用户可见状态语义不变。
3. 收口配置模型、部署入口和文档，统一改为 URL + API Key 的接法。

## 非目标

本次明确不做：

- 不改 MoviePilot 调用的业务时机，仍然只在审批通过时触发。
- 不改订阅状态机，不顺手实现 `INGESTED`、拒绝原因、用户侧 Telegram 结果通知。
- 不切换到 MoviePilot MCP 接口；本次只改现有 REST 集成的认证方式。
- 不保留长期双模式兼容；本次方案默认收口到单一 `X-API-KEY` 模式。

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：
  - `docs/system-architecture.md`
  - `docs/reference/configuration-reference.md`
  - `docs/runbooks/deployment-environment.md`
- 相关服务/页面/模型：
  - `services/api/internal/integrations/moviepilot/client.go`
  - `services/api/internal/config/config.go`
  - `services/api/internal/services/subscription/service.go`
  - `infrastructure/docker/docker-compose.yml`
- 当前行为：
  - `ApproveSubscription()` 触发 `MoviePilotClient.CreateSubscription()`。
  - `MoviePilotClient` 直接带 `X-API-KEY` 调用 `POST /api/v1/subscribe/`。
  - 配置中心当前暴露 `MOVIEPILOT_URL`、`MOVIEPILOT_API_KEY` 两个运行期配置项。
  - MoviePilot 联通测试使用 `GET /api/v1/site/` + `X-API-KEY`。
- 已完成收口：
  - 登录换 Bearer 的链路已经移除。
  - 设置中心、配置参考和部署文档已经统一改到 API Key 模式。
  - 旧版用户名/密码配置不再作为运行期兼容路径，只用于提示管理员迁移。

## 方案设计

### 1. 用户可见行为

- 新增能力：
  - 管理员在设置中心只需配置 `MoviePilot 地址 + API Key` 即可完成联通测试和审批下游同步。
- 修改现有行为：
  - 设置中心里的 MoviePilot 配置项从“用户名 / 密码”收口为单一 API Key。
  - 部署入口从 `MOVIEPILOT_USERNAME` / `MOVIEPILOT_PASSWORD` 改为 `MOVIEPILOT_API_KEY`。
- 哪些现有行为必须保持不变：
  - 用户提交订阅、管理员审批、Bot 通知和 Web 状态展示保持现状。
  - MoviePilot 调用失败时，订阅仍写为 `APPROVED`，错误继续落到 `mpError`。
  - MoviePilot 仍然是可选集成；未配置时跳过调用，不阻断主流程。

### 2. 数据与模型

> 本次不涉及数据模型变更。

- 运行期配置模型调整：
  - 保留 `MOVIEPILOT_URL`
  - 新增 `MOVIEPILOT_API_KEY`
  - 删除 `MOVIEPILOT_USERNAME`
  - 删除 `MOVIEPILOT_PASSWORD`
- `settings` 表不需要 schema 迁移：
  - 本次没有新增列、索引、约束
  - 历史遗留的 `MOVIEPILOT_USERNAME` / `MOVIEPILOT_PASSWORD` 键值可暂时留在表中但不再被定义和读取

### 3. 接口与边界

- 新增或修改哪些 API / Internal API / webhook / 命令：
  - 不改 Ember 对外 API 路径。
  - 改 Ember 到 MoviePilot 的外部调用方式：
    - 删除对 `POST /api/v1/login/access-token` 的依赖
    - 直接调用 `POST /api/v1/subscribe/`
    - 请求头改为 `X-API-KEY: <MOVIEPILOT_API_KEY>`
  - 配置测试改为使用 `GET /api/v1/site/` + `X-API-KEY`
- 请求参数与响应字段怎么变：
  - Ember 自身的订阅 API 出入参不变。
  - MoviePilot 订阅请求体沿用当前结构：`type`、`name`、`tmdbid`、`season`
  - 只更换鉴权头，不改业务 payload
- 哪些调用方会受影响：
  - API 的 `MoviePilotClient`
  - API 设置中心配置定义与测试逻辑
  - 运维部署脚本与环境变量
  - 设置中心页面显示的配置项内容会随配置定义自动变化，通常不需要单独改 Web 组件

### 4. 关键流程

按顺序写清主链路，不需要贴代码：

1. 管理员在设置中心填写 `MOVIEPILOT_URL` 和 `MOVIEPILOT_API_KEY`。
2. 配置测试调用 `GET {MOVIEPILOT_URL}/api/v1/site/`，请求头带 `X-API-KEY`，用于验证凭证是否可用。
3. 用户提交订阅，管理员批准后，`ApproveSubscription()` 继续调用 `MoviePilotClient.CreateSubscription()`。
4. `MoviePilotClient` 直接构造 `POST {MOVIEPILOT_URL}/api/v1/subscribe/` 请求，请求头带 `X-API-KEY`。
5. MoviePilot 返回成功时，Ember 将订阅更新为 `APPROVED`。
6. MoviePilot 返回错误或鉴权失败时，Ember 仍将订阅更新为 `APPROVED`，并把错误文本写入 `mpError`。

### 5. 失败路径与边界条件

- 只配置了 `MOVIEPILOT_URL`，未配置 `MOVIEPILOT_API_KEY`：视为 MoviePilot 未完整配置，联通测试给出明确错误。
- `MOVIEPILOT_API_KEY` 错误或过期：MoviePilot 返回 `401/403`，Ember 在 `mpError` 中保留原始失败信息，便于排障。
- MoviePilot 服务不可达或超时：沿用当前容错语义，不阻断审批主链路。
- 配置中心切换到新 key 后，旧的用户名密码条目仍残留在 `settings` 表：不读取、不展示、不作为兼容路径。
- 兼容性约束：
  - 不能改变 `ApproveSubscription()` 当前“下游失败不回滚审批状态”的行为。
  - 不能改动 Bot 通知、订阅去重、Webhook、TV Calendar 等无关链路。
  - 不能把这次改造扩大成 MCP 接口重构或多模式兼容工程。

## 影响范围

涉及的子系统：

- API：有
  - `MoviePilotClient` 鉴权方式
  - 配置中心定义与媒体配置测试逻辑
- Web：低影响
  - 设置中心通过现有配置定义自动展示新的 MoviePilot 字段，不预期需要改页面布局
- Bot：无
- 配置/部署：有
  - `infrastructure/docker/docker-compose.yml`
  - 运行环境变量说明
  - 现网配置项切换为 `MOVIEPILOT_API_KEY`
- 文档：需要更新
  - `docs/system-architecture.md`
  - `docs/reference/configuration-reference.md`
  - `docs/runbooks/deployment-environment.md`

## 验证方式

### 编译/测试

- `cd services/api && go test ./internal/integrations/moviepilot ./internal/config`
  - 2026-04-13 已执行，通过
- `cd services/api && go build ./...`
  - 2026-04-13 已执行，通过

按改动补充针对性测试：

- `MoviePilotClient` 单元测试：校验请求头从 Bearer 改为 `X-API-KEY`
- 配置测试：`MOVIEPILOT_URL + MOVIEPILOT_API_KEY` 成功，缺 key/错 key 失败

### 手工验证

- 在设置中心填写 `MOVIEPILOT_URL` 和有效 `MOVIEPILOT_API_KEY`，点击“测试媒体配置”，确认 MoviePilot 项返回成功
- 审批一条电影订阅，确认 MoviePilot 收到订阅且 Ember 状态更新为 `APPROVED`
- 审批一条电视剧订阅，确认季号仍被正确透传
- 故意填错 `MOVIEPILOT_API_KEY`，确认审批不被阻断，但 `mpError` 可见且日志能定位问题
- 确认未配置 MoviePilot 时，订阅审批行为与当前一致，不影响 Bot 通知和 Web 列表

## 落地后文档处理

本次归档前已同步处理：

- 将稳定结论同步到 `docs/system-architecture.md`
  - MoviePilotClient 的认证方式
  - 运行期配置项列表
- 更新 `docs/reference/configuration-reference.md` 和 `docs/runbooks/deployment-environment.md`
  - 删除用户名密码配置
  - 新增 `MOVIEPILOT_API_KEY`
- 当前代码、部署入口和文档都已完成切换，因此本方案移入 `docs/archive/plan/media-subscription/`
