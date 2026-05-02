# API 响应规范

> **版本**: v1.0
> **创建日期**: 2026-02-11
> **适用范围**: Ember 项目所有 REST API
> **维护者**: 后端团队

---

## 📌 规范目的

本规范定义了 Ember 项目中所有 REST API 的统一响应格式，确保：
- ✅ 前后端接口契约的一致性
- ✅ API 响应结构的可预测性
- ✅ 代码的可维护性和可扩展性
- ✅ 前端类型定义的准确性

---

## 🎯 核心原则

### 1. 一致性优先
所有同类型接口必须使用相同的响应结构，禁止出现以下情况：
```json
// ❌ 错误：同为列表接口但字段名不一致
GET /users  → { "users": [...] }
GET /invites → { "invites": [...] }

// ✅ 正确：统一使用 data 字段
GET /users  → { "data": [...], "total": 100 }
GET /invites → { "data": [...], "total": 50 }
```

### 2. 简洁性原则
避免过度嵌套和冗余字段：
```json
// ❌ 错误：不必要的嵌套
{
  "response": {
    "result": {
      "data": [...]
    }
  }
}

// ✅ 正确：扁平结构
{
  "data": [...],
  "total": 100
}
```

### 3. 可扩展性
预留扩展空间，便于未来添加字段：
```json
{
  "data": [...],
  "total": 100,
  "meta": {              // 未来可添加元数据
    "took": "15ms",
    "version": "v1"
  }
}
```

---

## 📋 响应格式分类

### 类型 1: 列表响应（带分页）

**使用场景**：返回多条记录，需要分页

**标准格式**：
```json
{
  "data": [...],        // 必需，数据列表
  "total": 100,         // 必需，总记录数
  "page": 1,            // 必需，当前页码
  "pageSize": 20,       // 必需，每页数量
  "totalPages": 5       // 可选，总页数
}
```

**Go 后端实现**：
```go
type ListResponse struct {
    Data       []YourModel `json:"data"`
    Total      int64       `json:"total"`
    Page       int         `json:"page"`
    PageSize   int         `json:"pageSize"`
    TotalPages int         `json:"totalPages,omitempty"`
}
```

**TypeScript 前端类型**：
```typescript
interface ListResponse<T> {
  data: T[]
  total: number
  page: number
  pageSize: number
  totalPages?: number
}
```

**适用接口**：
- `GET /admin/users` - 用户列表
- `GET /admin/invites` - 邀请码列表
- `GET /admin/subscriptions` - 订阅列表

---

### 类型 2: 单个实体响应

**使用场景**：返回单个对象或包含多个顶级字段

**标准格式**：
```json
{
  "token": "eyJhbGci...",
  "admin": {
    "id": "xxx",
    "username": "admin"
  }
}
```

**设计要点**：
- 不要用 `data` 包裹单个对象（过度嵌套）
- 直接使用语义明确的字段名
- 保持扁平结构

**适用接口**：
- `POST /admin/login` → `{ "token", "admin" }`
- `POST /user/register` → `{ "token", "user" }`
- `GET /user/profile` → 直接返回用户对象

### 认证与账号接口补充约定

以下行为已经稳定，后续实现与前端类型应以此为准：

#### 登录成功响应

`POST /api/v1/login`

```json
{
  "token": "eyJhbGciOi...",
  "user": {
    "id": "clxxxxx",
    "username": "ember",
    "role": "user",
    "passwordResetRequired": false
  },
  "isExpired": false,
  "passwordResetRequired": false
}
```

约束：

- `passwordResetRequired=true` 表示该账号必须先完成改密闭环，前端应优先跳转账号中心或改密页，而不是继续放行其他控制台入口。
- 登录失败统一返回与“用户名或密码错误”一致的错误语义，不得通过文案差异泄漏 EmbyID 错配、大小写碰撞或其他内部判定细节。

#### 忘记密码发送验证码

`POST /api/v1/forgot-password/send-code`

```json
{
  "message": "如果该邮箱已注册，验证码已发送"
}
```

约束：

- 除请求格式错误（400）和 SMTP 未配置导致服务不可用外，未注册邮箱、限流、发送失败等路径都应折叠为 `200 + 统一文案`。
- 不要通过状态码、错误文案或限流差异暴露“邮箱是否已注册”。

#### 通过验证码重置密码

`POST /api/v1/forgot-password/reset`

成功响应沿用操作结果或用户侧既有成功文案；失败时保持统一错误格式：

```json
{
  "error": "验证码无效或已过期"
}
```

约束：

- 验证码必须按 `email + code + type` 消费，成功后立即失效。
- `register` / `reset` / `change_email` 三类验证码不得混用。

#### 账号中心邮箱变更（两步流）

`POST /api/v1/email/send-code` / `POST /api/v1/user/email/send-code`

请求体：

```json
{
  "newEmail": "user@example.com"
}
```

成功响应（200）：

```json
{
  "message": "验证码已发送至新邮箱"
}
```

错误映射：

- 400：请求格式错误、`新邮箱与当前邮箱相同`（`ErrEmailUnchanged`）、`邮箱已被其他用户绑定`（`ErrEmailAlreadyBound`）、`邮箱已被注册`（`ErrEmailAlreadyRegistered`）
- 429：`该邮箱今日发送次数已达上限`（`ErrEmailCodeRateLimit`）/ `请求过于频繁，请稍后再试`（`ErrEmailCodeIPRateLimit`）
- 503：`邮件服务未配置`（`ErrEmailNotConfigured`）
- 500：其余内部错误统一走 `上游服务暂不可用`

`PUT /api/v1/email` / `PUT /api/v1/user/email`

请求体：

```json
{
  "newEmail": "user@example.com",
  "code": "123456"
}
```

成功响应（200）：返回更新后的 `User` 对象。后端会异步通知旧邮箱本次变更。

错误映射：

- 400：请求格式错误（`newEmail` 缺失或格式非法、`code` 长度不是 6）、`ErrEmailUnchanged`、`ErrEmailCodeInvalid`（验证码无效或已过期）、`ErrEmailAlreadyExists`
- 404：`ErrUserNotFound`
- 500：其余内部错误统一走 `上游服务暂不可用`

约束：

- `send-code` 业务前置失败（`Unchanged` / `AlreadyBound`）不消耗 `change_email` 限流配额，避免合法用户被对手用相同 / 占用邮箱反复打满
- 验证码必须发往**新邮箱**，并按 `change_email` 类型消费；不允许复用 `register` / `reset` 的码完成邮箱变更
- 旧邮箱通知是 fire-and-forget，前端不等待该副作用结果

---

### 类型 3: 操作结果响应

**使用场景**：执行操作后返回执行状态

**标准格式**：
```json
{
  "success": true,      // 必需，操作是否成功
  "message": "...",     // 可选，提示信息
  "count": 3,           // 可选，操作影响的记录数
  "errors": [...]       // 可选，错误详情列表
}
```

**Go 后端实现**：
```go
type OperationResult struct {
    Success bool     `json:"success"`
    Message string   `json:"message,omitempty"`
    Errors  []string `json:"errors,omitempty"`
}
```

**适用接口**：
- `POST /api/v1/cron/check-expired` → `{ "success": true, "disabledCount": 3 }`
- 某些 webhook / 后台批处理接口 → `{ "success": true, "message": "..." }`

说明：
- 管理员用户接口不应被机械归入这一类。当前 `PUT /api/v1/admin/users/:id/extend` / `toggle` 返回更新后的用户对象，`DELETE /api/v1/admin/users/:id` 返回 `{ "message": "删除成功" }`。
- 不要因为历史文档样例而默认新增接口必须返回 `success=true`。

---

### 类型 4: 错误响应

**统一错误格式**：
```json
{
  "error": "错误描述信息"
}
```

**HTTP 状态码映射**：
| 状态码 | 场景 | 示例 |
|--------|------|------|
| 200 | 成功 | 正常返回数据 |
| 400 | 请求参数错误 | `{"error": "用户名格式错误"}` |
| 401 | 未认证 | `{"error": "Token 无效或已过期"}` |
| 403 | 权限不足 | `{"error": "需要管理员权限"}` |
| 404 | 资源不存在 | `{"error": "用户不存在"}` |
| 500 | 内部错误 / 上游不可用 | `{"error": "上游服务暂不可用"}` |

**Go 错误处理示例**：
```go
// 统一错误响应
c.JSON(http.StatusBadRequest, gin.H{
    "error": "用户名格式错误",
})
```

### 设置中心配置对象补充语义

`GET /api/v1/admin/configs` / `GET /api/v1/internal/settings/:key` 返回的 `ConfigItem` 额外约定如下：

```json
{
  "key": "EMBY_API_KEY",
  "source": "database",
  "hasValue": true,
  "maskedValue": "********abcd"
}
```

字段语义：

- `source`: 当前值来源，取值如 `database` / `env` / `default` / `unset`
- `hasValue`: 当前配置是否有有效值；对敏感项即使不回显明文，也必须能看出“是否已设置”
- `maskedValue`: 仅敏感项返回，用于提示“已有值但不回显明文”；前端只用于展示，不可反解真实值

约束：

- 敏感项默认不回显 `value`
- 非敏感项可直接返回 `value`
- handler 不要自己拼“已设置/未设置”文案，前端应基于 `source/hasValue/maskedValue` 渲染

---

## 🔤 命名规范

### 字段命名分层规则

**规则**：
- JSON 字段统一使用 `camelCase`
- Go / GORM 结构体字段统一使用 `CamelCase`
- 数据库表名、列名、索引名统一使用 `snake_case`

历史上已经存在的 camelCase 数据库列属于遗留结构；新增表 / 新增列 / 新增索引禁止继续沿用这类命名。

### JSON 字段命名：camelCase

**规则**：所有 JSON 字段使用驼峰命名法

```json
{
  "userId": "xxx",        // ✅ 正确
  "user_id": "xxx",       // ❌ 错误（蛇形）
  "createdAt": "...",     // ✅ 正确
  "created_at": "...",    // ❌ 错误
  "isActive": true,       // ✅ 正确
  "is_active": true       // ❌ 错误
}
```

**原因**：
- JavaScript/TypeScript 的标准命名风格
- 与前端代码风格一致
- 无需字段名转换

**Go 字段、JSON 字段与数据库列映射**：
```go
// Go GORM 模型（必须显式指定 column）
type User struct {
    EmbyID    string    `json:"embyId" gorm:"column:emby_id"`
    CreatedAt time.Time `json:"createdAt" gorm:"column:created_at"`
}
```

⚠️ **重要**：
- 即使数据库当前已有遗留 camelCase 列，新增设计仍然按 `snake_case` 执行
- GORM 模型一律显式写 `gorm:"column:xxx"`，不要依赖默认推导，更不要从历史遗留列名反向复制新命名

---

## 📅 时间格式规范

### ISO 8601 + UTC

**标准格式**：`YYYY-MM-DDTHH:mm:ssZ`

```json
{
  "createdAt": "2026-02-11T10:30:00Z",    // ✅ 正确（UTC，末尾带Z）
  "expiresAt": "2027-01-15T00:00:00Z",

  "createdAt": "2026-02-11 10:30:00",     // ❌ 错误（非ISO格式）
  "createdAt": "2026-02-11T18:30:00+08:00" // ⚠️ 避免（时区信息）
}
```

**后端实现**：
```go
// Go 使用 UTC 时间
time.Now().UTC()

// GORM 配置
DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
    NowFunc: func() time.Time {
        return time.Now().UTC()
    },
})
```

**前端处理**：
```typescript
// 前端负责转换为本地时区
const date = new Date(user.createdAt)  // 自动转为本地时间
date.toLocaleString()  // "2026/2/11 下午6:30:00"（中国时区）
```

**时区原则**：
- 后端存储和返回：统一 UTC
- 前端显示：转换为用户本地时区
- 不要在后端做时区转换

---

## 🔢 数据类型规范

### 布尔值

```json
{
  "isActive": true,       // ✅ 正确（JSON boolean）
  "isActive": "true",     // ❌ 错误（字符串）
  "isActive": 1           // ❌ 错误（数字）
}
```

### 空值

**可选字段使用 `null`**：
```json
{
  "note": null,           // ✅ 推荐（明确表示为空）
  "note": "",             // ⚠️ 可接受（空字符串）
  // 省略 note 字段       // ❌ 避免（前端需额外判断）
}
```

**Go 实现**：
```go
type Model struct {
    Note *string `json:"note"` // 指针类型，可为 null
}
```

### 数字类型

```json
{
  "count": 42,            // ✅ 正确（number）
  "count": "42",          // ❌ 错误（字符串）
  "price": 19.99,         // ✅ 正确（浮点数）
  "page": 1               // ✅ 正确（整数）
}
```

---

## ✅ 前后端类型同步

### 强制要求

1. **Go 结构体必须包含 `json` tag**
2. **`json` tag 必须与前端 TypeScript 类型完全一致**
3. **新增接口必须先定义类型**

### 示例对比

**Go 后端**：
```go
type GetUsersResponse struct {
    Data       []models.User `json:"data"`
    Total      int64         `json:"total"`
    Page       int           `json:"page"`
    PageSize   int           `json:"pageSize"`
}
```

**TypeScript 前端**：
```typescript
interface UserListResponse {
  data: UserInfo[]
  total: number
  page: number
  pageSize: number
}
```

**一致性检查**：
- ✅ `data` ↔ `data`
- ✅ `total` ↔ `total`
- ✅ `page` ↔ `page`
- ✅ `pageSize` ↔ `pageSize`

---

## 🚨 常见错误

### 错误 1：列表字段名不统一
```json
// ❌ 错误
GET /users → { "users": [...] }
GET /invites → { "invites": [...] }

// ✅ 正确
GET /users → { "data": [...] }
GET /invites → { "data": [...] }
```

### 错误 2：GORM 模型未指定列名
```go
// ❌ 错误（新增数据库列规范是 snake_case，但这里没显式锁定映射）
type User struct {
    EmbyID string `json:"embyId"`
}

// ✅ 正确
type User struct {
    EmbyID string `json:"embyId" gorm:"column:emby_id"`
}
```

### 错误 3：时间格式不规范
```json
// ❌ 错误
"createdAt": "2026-02-11 10:30:00"

// ✅ 正确
"createdAt": "2026-02-11T10:30:00Z"
```

### 错误 4：过度嵌套
```json
// ❌ 错误
{
  "data": {
    "user": {
      "id": "xxx"
    }
  }
}

// ✅ 正确（单个实体）
{
  "id": "xxx",
  "username": "admin"
}
```

---

## 📋 新增接口检查清单

在开发新接口时，请按以下清单逐项检查：

### 后端开发
- [ ] 确定接口类型（列表/单个实体/操作结果）
- [ ] 定义 Go 响应结构体，包含正确的 `json` tag
- [ ] 列表接口使用 `data` 字段
- [ ] 时间字段返回 UTC ISO 8601 格式
- [ ] 错误响应包含 `error` 字段
- [ ] HTTP 状态码使用正确
- [ ] GORM 模型字段包含 `column:xxx` tag（如果使用 Prisma 数据库）

### 前端开发
- [ ] 定义 TypeScript 接口类型
- [ ] 字段名与后端 `json` tag 完全一致
- [ ] 时间字段类型为 `string`（不是 `Date`）
- [ ] 可选字段使用 `?` 标记
- [ ] 更新 API 调用代码

### 文档更新
- [ ] 更新 `docs/archive/reference/api-reference.md`
- [ ] 添加请求/响应示例
- [ ] 说明必需参数和可选参数

---

## 🔧 工具推荐

### 1. JSON 格式验证
```bash
# 使用 jq 验证 JSON 格式
echo '{"data":[]}' | jq .
```

### 2. 类型生成工具
- Go → TypeScript: 考虑使用 `tygo` 自动生成 TS 类型
- OpenAPI/Swagger: 考虑引入 Swagger 文档生成

### 3. API 测试
```bash
# 使用 curl 测试接口
curl -s http://localhost:8080/api/v1/admin/users \
  -H "Authorization: Bearer xxx" | jq .
```

---

## 📚 参考资料

### 业界标准
- [JSON:API Specification](https://jsonapi.org/)
- [Google JSON Style Guide](https://google.github.io/styleguide/jsoncstyleguide.xml)
- [Microsoft REST API Guidelines](https://github.com/microsoft/api-guidelines)

### 内部文档
- [旧版 API 参考](../archive/reference/api-reference.md) - 历史接口文档，仅供追溯
- [开发指南](./development-guide.md) - 当前开发入口

---

## 📝 版本历史

### v1.2 (2026-05-02)
- ✅ 增补账号中心邮箱变更两步流（`send-code` + `PUT /email`）的请求体、响应与错误码映射
- ✅ 验证码类型补 `change_email`，明确与 `register` / `reset` 配额隔离

### v1.1 (2026-05-02)
- ✅ 明确命名分层：数据库 `snake_case`，Go/GORM `CamelCase`，JSON `camelCase`
- ✅ 明确历史 camelCase 数据库列属于遗留结构，新增设计禁止继续扩散

### v1.0 (2026-02-11)
- ✅ 初始版本
- ✅ 定义列表、单个实体、操作结果三种响应格式
- ✅ 统一使用 `data` 字段包裹列表数据
- ✅ 规范字段命名（camelCase）、时间格式（ISO 8601 UTC）
- ✅ 明确前后端类型同步要求

---

**文档维护者**: 后端团队
**最后更新**: 2026-05-02
**如有疑问，请提交 Issue 或联系团队**
