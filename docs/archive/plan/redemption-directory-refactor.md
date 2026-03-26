# redemption 目录化方案

> 状态：已完成
> 负责人：Ember
> 更新时间：2026-03-26

## 背景

这个问题为什么当时要解决：

- `redemption` 与 `redemption_code` 曾长期平铺在根 `services`，和目录化方向不一致。
- 当时错误、请求类型、handler 调用面仍挂在根 `services`，会在后续 compat 清理阶段反复制造依赖回流。
- 本方案用于完成兑换领域的最终收口，并为后续 compat 清理提供前置条件。

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

以当前代码和现行文档为准，写清完成后的现状：

- 相关文档：`docs/reference/api-development-conventions.md`、`docs/proposals/api-directory-refactor.md`、`docs/system-architecture.md`
- 相关服务/页面/模型：
  - `services/api/internal/services/redemption/service.go`
  - `services/api/internal/services/redemption/code_service.go`
  - `services/api/internal/services/redemption/errors.go`
  - `services/api/internal/services/redemption/types.go`
  - `services/api/internal/services/auth.go`
  - `services/api/internal/handlers/user.go`
  - `services/api/internal/handlers/redemption_code.go`
  - `services/api/internal/handlers/telegram.go`
  - `services/api/internal/models/redemption.go`
  - `services/api/internal/models/redemption_code.go`
- 当前行为：兑换续期、邀请码注册校验、兑换码管理、Telegram 续期兑换都保持原行为不变，但实现入口已切到 `services/redemption/` 子目录。
- 已完成项：
  - `redemption` 已收口到 `services/redemption/`
  - 错误、类型、实现已一起迁入子包
  - `auth`、`handlers/user.go`、`handlers/redemption_code.go`、`handlers/telegram.go` 等调用面已切到子包
  - 未引入新的 `redemption_compat.go`
  - `go test ./...` 与 `go build ./...` 已通过
  - 相关现行文档已同步

## 方案设计

### 1. 用户可见行为

- 不新增用户能力。
- 不修改现有 API 路由、请求字段、响应字段、错误语义和状态码映射。
- 以下行为保持不变：
  - 用户兑换续期码
  - 邀请注册校验兑换码
  - 管理员创建/批量创建/编辑/删除兑换码
  - 管理员查看兑换历史
  - Telegram Bot 通过兑换码续期

### 2. 数据与模型

> 本次不涉及数据模型变更。

### 3. 接口与边界

新的内部目录边界采用单一领域包：

```text
services/api/internal/services/redemption/
  service.go
  code_service.go
  errors.go
  types.go
```

- `auth`、`handlers/user.go`、`handlers/redemption_code.go`、`handlers/telegram.go`、Telegram 默认装配链路直接依赖 `redemption` 子包，不再通过根 `services` 中转。
- 根 `services` 不再承载兑换领域的真实实现。
- 实际落地中未引入 `redemption_compat.go`。

### 4. 关键流程

按顺序完成了以下主链路：

1. 创建 `internal/services/redemption/`，迁入错误定义与请求/响应类型。
2. 迁移 `RedemptionService` 与 `RedemptionCodeService` 实现，保证新包不反向依赖根 `services`。
3. 修改 `auth`、`handlers/user.go`、`handlers/redemption_code.go`、`handlers/telegram.go` 等直接调用方。
4. 修改测试 stub 与错误断言，统一依赖 `redemption` 子包。
5. 更新 `docs/system-architecture.md` 与目录重构相关文档。
6. 全量执行 `go test ./...` 与 `go build ./...`。

### 5. 失败路径与边界条件

- 若新包继续引用根 `services` 中的错误或类型：会形成新的包级回流。
- 若只迁移实现、不迁移 handler 与测试导入：会留下半套调用面。
- 若 Telegram 默认装配链路仍通过根 `services` 调兑换：目录虽然看起来拆开，但依赖图没有真正收口。
- 兼容性约束：不能破坏当前注册邀请模式、兑换成功后延长有效期、模板用户校验、Telegram 续期兑换。

## 影响范围

涉及的子系统：

- API：有，涉及兑换域目录结构、handler 导入、`auth` 编排、Telegram 适配链路。
- Web：无，接口与字段保持不变。
- Bot：无直接改动，但依赖 API 内部调用面调整。
- 配置/部署：无。
- 文档：`docs/system-architecture.md`、`docs/reference/api-development-conventions.md`、`docs/proposals/api-directory-refactor.md`

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

落地后已同步处理：

- 稳定后的目录位置和服务说明已更新到 `docs/system-architecture.md`。
- 目录治理结论已同步到 `docs/reference/api-development-conventions.md` 与 `docs/proposals/api-directory-refactor.md`。
- 本方案已移入 `docs/archive/`。
