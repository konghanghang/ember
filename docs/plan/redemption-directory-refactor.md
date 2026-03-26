# redemption 目录化方案

> 状态：草稿
> 负责人：Codex
> 更新时间：2026-03-26

## 背景

这个问题为什么现在要解决：

- `services/api/internal/services/redemption.go` 与 `services/api/internal/services/redemption_code.go` 仍然平铺在根 `services`，和当前目录化方向不一致。
- 这两块业务已经形成稳定领域边界，但错误、请求类型、handler 调用面仍然挂在根 `services`，导致后续继续清理 `compat` 时会反复碰到依赖回流。
- 如果继续放着不处理，`services` 根层会长期同时承载“真实实现 + 过渡导出 + 共享错误”，目录重构永远无法收口。

## 目标

本方案要实现：

1. 将兑换相关能力收口到 `internal/services/redemption/` 单一领域目录。
2. 保持现有 HTTP API、Bot 行为和用户可见语义不变，只调整包结构与依赖方向。
3. 为后续删除 `compat` 文件创造前置条件，避免再次出现“拆了目录但调用面还在根层”的假重构。

## 非目标

本次明确不做：

- 不改动兑换码规则、注册规则、一人一码约束、模板用户策略。
- 不顺手拆 `auth`、`user`、`email`。
- 不修改数据库 schema、API 路由、响应字段或前端页面结构。

## 当前事实

以当前代码和现行文档为准，先写清现状：

- 相关文档：`docs/reference/api-development-conventions.md`、`docs/proposals/api-directory-refactor.md`、`docs/system-architecture.md`
- 相关服务/页面/模型：
  - `services/api/internal/services/redemption.go`
  - `services/api/internal/services/redemption_code.go`
  - `services/api/internal/services/redemption_errors.go`
  - `services/api/internal/services/auth.go`
  - `services/api/internal/handlers/user.go`
  - `services/api/internal/handlers/redemption_code.go`
  - `services/api/internal/handlers/telegram.go`
  - `services/api/internal/services/telegram_compat.go`
  - `services/api/internal/models/redemption.go`
  - `services/api/internal/models/redemption_code.go`
- 当前行为：兑换续期、邀请码注册校验、兑换码管理、Telegram 续期兑换都能工作，但它们的实现入口仍依赖根 `services` 包。
- 现有限制：
  - 共享错误仍定义在根 `services/redemption_errors.go`
  - handler 和测试仍直接依赖 `services.*` 类型
  - `telegram_compat.go` 通过根 `services` 反向调用兑换实现，进一步放大目录重构时的回流风险

## 方案设计

### 1. 用户可见行为

- 不新增用户能力。
- 不修改现有 API 路由、请求字段、响应字段、错误语义和状态码映射。
- 以下行为必须保持不变：
  - 用户兑换续期码
  - 邀请注册校验兑换码
  - 管理员创建/批量创建/编辑/删除兑换码
  - 管理员查看兑换历史
  - Telegram Bot 通过兑换码续期

### 2. 数据与模型

> 本次不涉及数据模型变更。

### 3. 接口与边界

- 新的内部目录边界采用单一领域包，而不是拆成两个 sibling 包：

```text
services/api/internal/services/redemption/
  service.go        # RedemptionService，负责兑换续期与兑换历史
  code_service.go   # RedemptionCodeService，负责兑换码 CRUD/校验/模板用户
  errors.go         # 兑换领域错误
  types.go          # 兑换与兑换码请求/响应结构
```

- `auth`、`handlers/user.go`、`handlers/redemption_code.go`、`handlers/telegram.go`、`services/telegram_compat.go` 直接依赖 `redemption` 子包，不再通过根 `services` 中转。
- 根 `services` 不再承载兑换领域的真实实现。
- 是否保留 `redemption_compat.go`：
  - 默认方案：同一轮直接改完所有生产调用方，不新增 compat。
  - 退让方案：若改动面超出单轮可控范围，只允许加入一个极短期 `redemption_compat.go`，并在同一份计划里明确删除步骤；不得长期保留。

### 4. 关键流程

按顺序写清主链路，不需要贴代码：

1. 先创建 `internal/services/redemption/`，把错误定义与请求/响应类型整体迁入子包。
2. 再迁移 `RedemptionService` 与 `RedemptionCodeService` 的实现文件，保证新包只依赖 `db/models/integrations/common`，不反向依赖根 `services`。
3. 修改直接调用方：
   - `services/api/internal/services/auth.go`
   - `services/api/internal/handlers/user.go`
   - `services/api/internal/handlers/redemption_code.go`
   - `services/api/internal/handlers/telegram.go`
   - `services/api/internal/services/telegram_compat.go`
4. 修改测试中的 stub 和错误断言，统一改为依赖 `redemption` 子包。
5. 更新 `docs/system-architecture.md` 与目录重构相关文档，记录新的目录位置和依赖约束。
6. 全量执行 `go test ./...` 与 `go build ./...`，确认没有循环依赖、没有旧导入残留。

### 5. 失败路径与边界条件

- 若新包继续引用根 `services` 中的错误或类型：会形成新的包级回流，必须立即回退该步改法，先把共享定义迁入子包再继续。
- 若只迁移实现、不迁移 handler 与测试导入：会留下半套调用面，后续兼容层会继续掩盖旧依赖，不能接受。
- 若 `telegram_compat.go` 仍通过根 `services` 调兑换：目录虽然看起来拆开，但依赖图没有真正收口，必须同步改掉。
- 兼容性约束：不能破坏当前注册邀请模式、兑换成功后延长有效期、模板用户校验、Telegram 续期兑换。

## 影响范围

涉及的子系统：

- API：有，涉及兑换域目录结构、handler 导入、`auth` 编排、Telegram 适配层。
- Web：无，接口与字段保持不变。
- Bot：无直接改动，但会受 API 内部依赖调整影响，需要保证 Telegram 兑换路径行为不变。
- 配置/部署：无。
- 文档：需更新 `docs/system-architecture.md`；若最终确定新的目录治理结论，补充到 `docs/reference/api-development-conventions.md` 或同步 `docs/proposals/api-directory-refactor.md` 的进度。

## 验证方式

### 编译/测试

- `cd services/api && go test ./...`
- `cd services/api && go build ./...`

### 手工验证

- 邀请注册模式下，使用有效兑换码完成注册，确认注册成功且兑换码使用次数正确增加。
- 用户中心兑换续期码，确认有效期延长、兑换历史落库、已过期 Emby 用户可正常解封。
- 管理后台创建、批量创建、编辑、删除兑换码，确认状态筛选和模板用户行为不变。
- Telegram Bot 通过兑换码续期，确认错误映射和成功响应不变。

## 落地后文档处理

落地后应同步处理：

- 将稳定后的目录位置和服务说明更新到 `docs/system-architecture.md`。
- 若本次确认了“目录化时必须先迁错误与类型，再迁实现”的稳定规则，提炼到 `docs/reference/api-development-conventions.md`。
- 实施完成并稳定后，将本方案移入 `docs/archive/`。
