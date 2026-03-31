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
- `PUT /admin/users/:id/extend` → `{ "success": true }`
- `DELETE /admin/users/:id` → `{ "success": true }`
- `POST /admin/cron/check-expired` → `{ "success": true, "disabledCount": 3 }`

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
| 500 | 服务器错误 | `{"error": "数据库连接失败"}` |

**Go 错误处理示例**：
```go
// 统一错误响应
c.JSON(http.StatusBadRequest, gin.H{
    "error": "用户名格式错误",
})
```

---

## 🔤 命名规范

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

**数据库列名与 JSON 字段映射**：
```go
// Prisma Schema（驼峰命名）
model User {
  embyId    String
  createdAt DateTime
}

// Go GORM 模型（必须显式指定 column）
type User struct {
    EmbyID    string    `json:"embyId" gorm:"column:embyId"`
    CreatedAt time.Time `json:"createdAt" gorm:"column:createdAt"`
}
```

⚠️ **重要**：GORM 默认将字段转换为蛇形命名（`EmbyID` → `emby_id`），而 Prisma 使用驼峰命名，因此必须使用 `gorm:"column:xxx"` 显式指定列名。

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
// ❌ 错误（GORM 会转为 emby_id，但数据库列名是 embyId）
type User struct {
    EmbyID string `json:"embyId"`
}

// ✅ 正确
type User struct {
    EmbyID string `json:"embyId" gorm:"column:embyId"`
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

### v1.0 (2026-02-11)
- ✅ 初始版本
- ✅ 定义列表、单个实体、操作结果三种响应格式
- ✅ 统一使用 `data` 字段包裹列表数据
- ✅ 规范字段命名（camelCase）、时间格式（ISO 8601 UTC）
- ✅ 明确前后端类型同步要求

---

**文档维护者**: 后端团队
**最后更新**: 2026-02-11
**如有疑问，请提交 Issue 或联系团队**
