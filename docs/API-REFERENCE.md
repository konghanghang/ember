# Ember API 接口文档

> **版本**: v1.0
> **基础路径**: `http://localhost:8080/api/v1`
> **认证方式**: JWT Bearer Token

---

## 📋 目录

- [响应规范](#响应规范)
- [1. 认证相关 (7个)](#1-认证相关)
- [2. 用户管理 - 管理员 (6个)](#2-用户管理---管理员)
- [3. 邀请码管理 (4个)](#3-邀请码管理)
- [4. 订阅管理 (6个)](#4-订阅管理)
- [5. 用户面板 (4个)](#5-用户面板)
- [6. 媒体相关 (2个)](#6-媒体相关)
- [7. 系统相关 (2个)](#7-系统相关)
- [8. 定时任务 (1个)](#8-定时任务)
- [9. 工具接口 (1个)](#9-工具接口)

---

## 📐 响应规范

为了保持 API 响应的一致性和可维护性，本项目遵循以下统一的响应格式规范。

### 1. 列表类接口（带分页）

**适用场景**: 返回多条记录，需要分页的接口

**响应格式**:
```json
{
  "data": [...],        // 数据列表，统一使用 data 字段
  "total": 100,         // 总记录数
  "page": 1,            // 当前页码
  "pageSize": 20,       // 每页数量
  "totalPages": 5       // 总页数（可选）
}
```

**示例接口**:
- `GET /admin/users` - 用户列表
- `GET /admin/invites` - 邀请码列表
- `GET /admin/subscriptions` - 订阅列表

**设计原因**:
- ✅ **一致性**: 所有列表接口统一使用 `data` 字段包裹数据
- ✅ **可扩展性**: 便于添加元数据（如分页信息、统计信息）
- ✅ **行业标准**: 符合 JSON:API、GraphQL 等主流规范
- ✅ **类型安全**: 前端可使用泛型 `ListResponse<T>` 统一处理

---

### 2. 单个实体接口

**适用场景**: 返回单个对象或包含多个字段的复杂响应

**响应格式**:
```json
{
  "token": "...",
  "user": {...},
  "otherField": "..."
}
```

**示例接口**:
- `POST /admin/login` - 登录响应（token + admin）
- `POST /user/register` - 注册响应（token + user）
- `GET /admin/users/:id` - 单个用户详情

**设计原因**:
- ✅ **语义清晰**: 字段名直接表达含义
- ✅ **扁平结构**: 减少不必要的嵌套
- ✅ **向后兼容**: 易于添加新字段

---

### 3. 操作结果接口

**适用场景**: 执行操作后返回状态信息

**响应格式**:
```json
{
  "success": true,
  "message": "操作成功",
  "disabledCount": 3,
  "errors": [...]
}
```

**示例接口**:
- `POST /admin/cron/check-expired` - 定时任务结果
- `POST /admin/system/test-emby` - Emby 连接测试
- `PUT /admin/users/:id/extend` - 延长用户到期时间

**设计原因**:
- ✅ **明确状态**: `success` 字段明确表示操作是否成功
- ✅ **附加信息**: 可包含操作统计、错误详情等
- ✅ **错误处理**: `errors` 数组可记录部分失败的情况

---

### 4. 错误响应

**统一错误格式**:
```json
{
  "error": "错误描述信息"
}
```

**HTTP 状态码规范**:
| 状态码 | 说明 | 示例 |
|--------|------|------|
| 200 | 成功 | 正常返回数据 |
| 400 | 请求参数错误 | `{"error": "用户名格式错误"}` |
| 401 | 未认证 | `{"error": "Token 无效或已过期"}` |
| 403 | 权限不足 | `{"error": "需要管理员权限"}` |
| 404 | 资源不存在 | `{"error": "用户不存在"}` |
| 500 | 服务器错误 | `{"error": "数据库连接失败"}` |

---

### 5. 字段命名规范

**JSON 字段命名**: 使用 **camelCase（驼峰命名）**

```json
{
  "userId": "xxx",        ✅ 正确
  "user_id": "xxx",       ❌ 错误（蛇形命名）
  "createdAt": "...",     ✅ 正确
  "created_at": "...",    ❌ 错误
}
```

**原因**:
- JavaScript/TypeScript 的标准命名风格
- 与前端代码风格保持一致
- 无需额外的字段名转换

---

### 6. 时间格式规范

**统一使用 ISO 8601 格式**:
```json
{
  "createdAt": "2026-02-11T10:30:00Z",      // UTC 时间
  "expiresAt": "2027-01-15T23:59:59Z"
}
```

**时区要求**:
- 后端存储和返回均使用 **UTC 时间**（末尾带 `Z`）
- 前端负责转换为用户本地时区显示

---

### 7. 布尔值规范

**直接使用 JSON boolean**:
```json
{
  "isActive": true,       ✅ 正确
  "isActive": "true",     ❌ 错误（字符串）
  "isActive": 1           ❌ 错误（数字）
}
```

---

### 8. 空值处理

**可选字段的空值表示**:
```json
{
  "note": null,           ✅ 推荐（明确表示为空）
  "note": ""              ⚠️ 可接受（空字符串）
  // "note" 省略          ❌ 避免（前端需要额外判断）
}
```

**建议**:
- 可选字段返回 `null` 而不是省略
- 前端可以统一处理 `null` 值

---

### 9. 前后端类型对齐

**Go 后端结构体示例**:
```go
type GetUsersResponse struct {
    Data       []models.User `json:"data"`       // 统一使用 data
    Total      int64         `json:"total"`
    Page       int           `json:"page"`
    PageSize   int           `json:"pageSize"`
    TotalPages int           `json:"totalPages"`
}
```

**TypeScript 前端类型示例**:
```typescript
interface UserListResponse {
  data: UserInfo[]
  total: number
  page: number
  pageSize: number
  totalPages?: number
}
```

**强制要求**:
- Go 结构体的 `json` tag 必须与前端 TypeScript 类型完全一致
- 新增接口必须先定义类型，再实现代码
- 修改响应结构需同步更新前后端类型定义

---

### 10. 规范检查清单

**新增接口时请确认**:
- [ ] 列表接口使用 `data` 字段
- [ ] 分页信息包含 `total`, `page`, `pageSize`
- [ ] 字段名使用 camelCase
- [ ] 时间字段使用 ISO 8601 UTC 格式
- [ ] 错误响应包含 `error` 字段
- [ ] HTTP 状态码使用正确
- [ ] 前后端类型定义已同步

---

## 1. 认证相关

### 1.1 管理员登录

**接口**: `POST /admin/login`
**认证**: 无需认证
**描述**: 管理员登录，返回 JWT Token

#### 请求参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| username | string | 是 | 管理员用户名 |
| password | string | 是 | 管理员密码 |

#### 请求示例

```bash
curl -X POST http://localhost:8080/api/v1/admin/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "admin",
    "password": "password123"
  }'
```

#### 响应示例

```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "admin": {
    "id": "clxxxx",
    "username": "admin",
    "createdAt": "2026-01-01T00:00:00Z"
  }
}
```

---

### 1.2 获取当前管理员信息

**接口**: `GET /admin/current`
**认证**: Bearer Token (Admin)
**描述**: 获取当前登录管理员的信息

#### 请求示例

```bash
curl http://localhost:8080/api/v1/admin/current \
  -H "Authorization: Bearer {admin_token}"
```

#### 响应示例

```json
{
  "id": "clxxxx",
  "username": "admin",
  "createdAt": "2026-01-01T00:00:00Z"
}
```

---

### 1.3 管理员登出

**接口**: `POST /admin/logout`
**认证**: Bearer Token (Admin)
**描述**: 管理员登出（前端清除 Token）

#### 响应示例

```json
{
  "message": "登出成功"
}
```

---

### 1.4 用户登录

**接口**: `POST /user/login`
**认证**: 无需认证
**描述**: 用户登录，验证 Emby 密码，返回 JWT Token

#### 请求参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| username | string | 是 | 用户名 |
| password | string | 是 | 密码 |

#### 请求示例

```bash
curl -X POST http://localhost:8080/api/v1/user/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "testuser",
    "password": "password123"
  }'
```

#### 响应示例

```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": {
    "id": "clxxxx",
    "username": "testuser",
    "email": "test@example.com",
    "embyId": "emby-user-id",
    "expiresAt": "2027-01-01T00:00:00Z",
    "isActive": true
  }
}
```

---

### 1.5 用户登出

**接口**: `POST /user/logout`
**认证**: Bearer Token (User)
**描述**: 用户登出（前端清除 Token）

#### 响应示例

```json
{
  "message": "登出成功"
}
```

---

### 1.6 用户注册

**接口**: `POST /user/register`
**认证**: 无需认证
**描述**: 使用邀请码注册用户，创建 Emby 账号

#### 请求参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| username | string | 是 | 用户名（3-50字符） |
| password | string | 是 | 密码（6-50字符） |
| email | string | 是 | 邮箱 |
| inviteCode | string | 是 | 邀请码 |

#### 请求示例

```bash
curl -X POST http://localhost:8080/api/v1/user/register \
  -H "Content-Type: application/json" \
  -d '{
    "username": "newuser",
    "password": "password123",
    "email": "new@example.com",
    "inviteCode": "INVITE123"
  }'
```

#### 响应示例

```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": {
    "id": "clxxxx",
    "username": "newuser",
    "email": "new@example.com",
    "embyId": "emby-user-id",
    "expiresAt": "2027-01-01T00:00:00Z"
  }
}
```

---

### 1.7 验证邀请码

**接口**: `GET /invites/:code/validate`
**认证**: 无需认证
**描述**: 验证邀请码是否有效

#### 路径参数

| 参数名 | 类型 | 说明 |
|--------|------|------|
| code | string | 邀请码 |

#### 请求示例

```bash
curl http://localhost:8080/api/v1/invites/INVITE123/validate
```

#### 响应示例

```json
{
  "valid": true,
  "invite": {
    "code": "INVITE123",
    "maxUses": 5,
    "usedCount": 2,
    "defaultDays": 30,
    "expiresAt": "2027-01-01T00:00:00Z"
  }
}
```

---

## 2. 用户管理 - 管理员

### 2.1 获取用户列表

**接口**: `GET /admin/users`
**认证**: Bearer Token (Admin)
**描述**: 分页查询用户列表，支持搜索

#### 查询参数

| 参数名 | 类型 | 必填 | 默认值 | 说明 |
|--------|------|------|--------|------|
| page | integer | 否 | 1 | 页码 |
| pageSize | integer | 否 | 10 | 每页数量 |
| search | string | 否 | - | 搜索关键词（用户名/邮箱） |

#### 请求示例

```bash
curl "http://localhost:8080/api/v1/admin/users?page=1&pageSize=10&search=test" \
  -H "Authorization: Bearer {admin_token}"
```

#### 响应示例

```json
{
  "data": [
    {
      "id": "clxxxx",
      "username": "testuser",
      "email": "test@example.com",
      "embyId": "emby-id",
      "inviteCode": "INVITE123",
      "expiresAt": "2027-01-01T00:00:00Z",
      "isActive": true,
      "createdAt": "2026-01-01T00:00:00Z"
    }
  ],
  "total": 100,
  "page": 1,
  "pageSize": 10
}
```

---

### 2.2 获取用户详情

**接口**: `GET /admin/users/:id`
**认证**: Bearer Token (Admin)
**描述**: 获取单个用户的详细信息

#### 路径参数

| 参数名 | 类型 | 说明 |
|--------|------|------|
| id | string | 用户 ID |

#### 响应示例

```json
{
  "id": "clxxxx",
  "username": "testuser",
  "email": "test@example.com",
  "embyId": "emby-id",
  "inviteCode": "INVITE123",
  "expiresAt": "2027-01-01T00:00:00Z",
  "isActive": true,
  "createdAt": "2026-01-01T00:00:00Z",
  "updatedAt": "2026-01-01T00:00:00Z"
}
```

---

### 2.3 延长用户到期时间

**接口**: `PUT /admin/users/:id/extend`
**认证**: Bearer Token (Admin)
**描述**: 延长用户账号的到期时间

#### 路径参数

| 参数名 | 类型 | 说明 |
|--------|------|------|
| id | string | 用户 ID |

#### 请求参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| days | integer | 是 | 延长天数（1-365） |

#### 请求示例

```bash
curl -X PUT http://localhost:8080/api/v1/admin/users/clxxxx/extend \
  -H "Authorization: Bearer {admin_token}" \
  -H "Content-Type: application/json" \
  -d '{"days": 30}'
```

#### 响应示例

```json
{
  "success": true,
  "newExpiresAt": "2027-02-01T00:00:00Z"
}
```

---

### 2.4 启用/禁用用户

**接口**: `PUT /admin/users/:id/toggle`
**认证**: Bearer Token (Admin)
**描述**: 切换用户的启用/禁用状态

#### 路径参数

| 参数名 | 类型 | 说明 |
|--------|------|------|
| id | string | 用户 ID |

#### 请求示例

```bash
curl -X PUT http://localhost:8080/api/v1/admin/users/clxxxx/toggle \
  -H "Authorization: Bearer {admin_token}"
```

#### 响应示例

```json
{
  "success": true,
  "isActive": false
}
```

---

### 2.5 重置用户密码

**接口**: `PUT /admin/users/:id/reset-password`
**认证**: Bearer Token (Admin)
**描述**: 管理员重置用户密码

#### 路径参数

| 参数名 | 类型 | 说明 |
|--------|------|------|
| id | string | 用户 ID |

#### 请求参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| newPassword | string | 是 | 新密码（6-50字符） |

#### 请求示例

```bash
curl -X PUT http://localhost:8080/api/v1/admin/users/clxxxx/reset-password \
  -H "Authorization: Bearer {admin_token}" \
  -H "Content-Type: application/json" \
  -d '{"newPassword": "newpassword123"}'
```

#### 响应示例

```json
{
  "success": true
}
```

---

### 2.6 删除用户

**接口**: `DELETE /admin/users/:id`
**认证**: Bearer Token (Admin)
**描述**: 删除用户（同时删除 Emby 账号）

#### 路径参数

| 参数名 | 类型 | 说明 |
|--------|------|------|
| id | string | 用户 ID |

#### 请求示例

```bash
curl -X DELETE http://localhost:8080/api/v1/admin/users/clxxxx \
  -H "Authorization: Bearer {admin_token}"
```

#### 响应示例

```json
{
  "success": true
}
```

---

## 3. 邀请码管理

### 3.1 获取邀请码列表

**接口**: `GET /admin/invites`
**认证**: Bearer Token (Admin)
**描述**: 分页查询邀请码列表

#### 查询参数

| 参数名 | 类型 | 必填 | 默认值 | 说明 |
|--------|------|------|--------|------|
| page | integer | 否 | 1 | 页码 |
| pageSize | integer | 否 | 20 | 每页数量 |
| showAll | boolean | 否 | false | 是否显示已过期和已用完的邀请码 |

#### 请求示例

```bash
curl "http://localhost:8080/api/v1/admin/invites?page=1&pageSize=10" \
  -H "Authorization: Bearer {admin_token}"
```

#### 响应示例

```json
{
  "data": [
    {
      "id": "clxxxx",
      "code": "INVITE123",
      "maxUses": 5,
      "usedCount": 2,
      "defaultDays": 30,
      "expiresAt": "2027-01-01T00:00:00Z",
      "createdAt": "2026-01-01T00:00:00Z"
    }
  ],
  "total": 50,
  "page": 1,
  "pageSize": 10
}
```

---

### 3.2 创建邀请码

**接口**: `POST /admin/invites`
**认证**: Bearer Token (Admin)
**描述**: 创建新的邀请码

#### 请求参数

| 参数名 | 类型 | 必填 | 默认值 | 说明 |
|--------|------|------|--------|------|
| maxUses | integer | 否 | 1 | 最大使用次数 |
| defaultDays | integer | 否 | 30 | 默认有效天数 |
| expiresAt | string | 否 | null | 邀请码过期时间（ISO 8601） |

#### 请求示例

```bash
curl -X POST http://localhost:8080/api/v1/admin/invites \
  -H "Authorization: Bearer {admin_token}" \
  -H "Content-Type: application/json" \
  -d '{
    "maxUses": 5,
    "defaultDays": 30,
    "expiresAt": "2027-01-01T00:00:00Z"
  }'
```

#### 响应示例

```json
{
  "invite": {
    "id": "clxxxx",
    "code": "A1B2C3D4",
    "maxUses": 5,
    "usedCount": 0,
    "defaultDays": 30,
    "expiresAt": "2027-01-01T00:00:00Z",
    "createdAt": "2026-01-01T00:00:00Z"
  }
}
```

---

### 3.3 删除邀请码

**接口**: `DELETE /admin/invites/:id`
**认证**: Bearer Token (Admin)
**描述**: 删除邀请码（不能删除已使用的）

#### 路径参数

| 参数名 | 类型 | 说明 |
|--------|------|------|
| id | string | 邀请码 ID |

#### 请求示例

```bash
curl -X DELETE http://localhost:8080/api/v1/admin/invites/clxxxx \
  -H "Authorization: Bearer {admin_token}"
```

#### 响应示例

```json
{
  "success": true
}
```

---

## 4. 订阅管理

### 4.1 用户创建订阅

**接口**: `POST /user/subscriptions`
**认证**: Bearer Token (User)
**描述**: 用户提交订阅请求

#### 请求参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| type | string | 是 | 媒体类型（MOVIE/TV） |
| name | string | 是 | 影视名称 |
| tmdbId | string | 是 | TMDB ID |
| posterPath | string | 否 | 海报路径 |
| note | string | 否 | 用户备注 |

#### 请求示例

```bash
curl -X POST http://localhost:8080/api/v1/user/subscriptions \
  -H "Authorization: Bearer {user_token}" \
  -H "Content-Type: application/json" \
  -d '{
    "type": "MOVIE",
    "name": "黑客帝国",
    "tmdbId": "603",
    "posterPath": "/path/to/poster.jpg",
    "note": "想看很久了"
  }'
```

#### 响应示例

```json
{
  "success": true
}
```

---

### 4.2 用户查看订阅列表

**接口**: `GET /user/subscriptions`
**认证**: Bearer Token (User)
**描述**: 获取当前用户的所有订阅

#### 请求示例

```bash
curl http://localhost:8080/api/v1/user/subscriptions \
  -H "Authorization: Bearer {user_token}"
```

#### 响应示例

```json
[
  {
    "id": "clxxxx",
    "userId": "cluser",
    "type": "MOVIE",
    "name": "黑客帝国",
    "tmdbId": "603",
    "posterPath": "/path/to/poster.jpg",
    "status": "PENDING",
    "note": "想看很久了",
    "mpError": null,
    "createdAt": "2026-01-01T00:00:00Z"
  }
]
```

---

### 4.3 用户删除订阅

**接口**: `DELETE /user/subscriptions/:id`
**认证**: Bearer Token (User)
**描述**: 删除订阅（仅 PENDING 状态可删除）

#### 路径参数

| 参数名 | 类型 | 说明 |
|--------|------|------|
| id | string | 订阅 ID |

#### 请求示例

```bash
curl -X DELETE http://localhost:8080/api/v1/user/subscriptions/clxxxx \
  -H "Authorization: Bearer {user_token}"
```

#### 响应示例

```json
{
  "success": true
}
```

---

### 4.4 管理员查看所有订阅

**接口**: `GET /admin/subscriptions`
**认证**: Bearer Token (Admin)
**描述**: 分页查询所有订阅，支持状态筛选

#### 查询参数

| 参数名 | 类型 | 必填 | 默认值 | 说明 |
|--------|------|------|--------|------|
| status | string | 否 | - | 状态筛选（PENDING/APPROVED/REJECTED） |
| page | integer | 否 | 1 | 页码 |
| pageSize | integer | 否 | 10 | 每页数量 |

#### 请求示例

```bash
curl "http://localhost:8080/api/v1/admin/subscriptions?status=PENDING&page=1&pageSize=10" \
  -H "Authorization: Bearer {admin_token}"
```

#### 响应示例

```json
{
  "data": [
    {
      "id": "clxxxx",
      "userId": "cluser",
      "type": "MOVIE",
      "name": "黑客帝国",
      "tmdbId": "603",
      "posterPath": "/path/to/poster.jpg",
      "status": "PENDING",
      "note": "想看很久了",
      "mpError": null,
      "createdAt": "2026-01-01T00:00:00Z",
      "user": {
        "username": "testuser",
        "email": "test@example.com"
      }
    }
  ],
  "total": 50
}
```

---

### 4.5 管理员批准订阅

**接口**: `PUT /admin/subscriptions/:id/approve`
**认证**: Bearer Token (Admin)
**描述**: 批准订阅，调用 MoviePilot API

#### 路径参数

| 参数名 | 类型 | 说明 |
|--------|------|------|
| id | string | 订阅 ID |

#### 请求示例

```bash
curl -X PUT http://localhost:8080/api/v1/admin/subscriptions/clxxxx/approve \
  -H "Authorization: Bearer {admin_token}"
```

#### 响应示例

```json
{
  "success": true
}
```

**注意**: 如果 MoviePilot API 调用失败，订阅状态仍为 APPROVED，错误信息记录在 `mpError` 字段。

---

### 4.6 管理员拒绝订阅

**接口**: `PUT /admin/subscriptions/:id/reject`
**认证**: Bearer Token (Admin)
**描述**: 拒绝订阅

#### 路径参数

| 参数名 | 类型 | 说明 |
|--------|------|------|
| id | string | 订阅 ID |

#### 请求示例

```bash
curl -X PUT http://localhost:8080/api/v1/admin/subscriptions/clxxxx/reject \
  -H "Authorization: Bearer {admin_token}"
```

#### 响应示例

```json
{
  "success": true
}
```

---

## 5. 用户面板

### 5.1 获取个人信息

**接口**: `GET /user/profile`
**认证**: Bearer Token (User)
**描述**: 获取当前登录用户的个人信息

#### 请求示例

```bash
curl http://localhost:8080/api/v1/user/profile \
  -H "Authorization: Bearer {user_token}"
```

#### 响应示例

```json
{
  "id": "clxxxx",
  "username": "testuser",
  "email": "test@example.com",
  "embyId": "emby-id",
  "expiresAt": "2027-01-01T00:00:00Z",
  "isActive": true,
  "createdAt": "2026-01-01T00:00:00Z"
}
```

---

### 5.2 更新个人信息

**接口**: `PUT /user/profile`
**认证**: Bearer Token (User)
**描述**: 更新个人信息（目前仅支持邮箱）

#### 请求参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| email | string | 是 | 新邮箱 |

#### 请求示例

```bash
curl -X PUT http://localhost:8080/api/v1/user/profile \
  -H "Authorization: Bearer {user_token}" \
  -H "Content-Type: application/json" \
  -d '{"email": "newemail@example.com"}'
```

#### 响应示例

```json
{
  "success": true
}
```

---

### 5.3 修改密码

**接口**: `PUT /user/password`
**认证**: Bearer Token (User)
**描述**: 修改密码（需验证旧密码）

#### 请求参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| oldPassword | string | 是 | 旧密码 |
| newPassword | string | 是 | 新密码（6-50字符） |

#### 请求示例

```bash
curl -X PUT http://localhost:8080/api/v1/user/password \
  -H "Authorization: Bearer {user_token}" \
  -H "Content-Type: application/json" \
  -d '{
    "oldPassword": "oldpassword123",
    "newPassword": "newpassword456"
  }'
```

#### 响应示例

```json
{
  "success": true
}
```

---

### 5.4 修改邮箱

**接口**: `PUT /user/email`
**认证**: Bearer Token (User)
**描述**: 修改邮箱地址

#### 请求参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| newEmail | string | 是 | 新邮箱 |

#### 请求示例

```bash
curl -X PUT http://localhost:8080/api/v1/user/email \
  -H "Authorization: Bearer {user_token}" \
  -H "Content-Type: application/json" \
  -d '{"newEmail": "newemail@example.com"}'
```

#### 响应示例

```json
{
  "success": true
}
```

---

## 6. 媒体相关

### 6.1 获取 Emby 配置

**接口**: `GET /user/emby/config`
**认证**: Bearer Token (User)
**描述**: 获取 Emby 服务器地址

#### 请求示例

```bash
curl http://localhost:8080/api/v1/user/emby/config \
  -H "Authorization: Bearer {user_token}"
```

#### 响应示例

```json
{
  "success": true,
  "url": "https://emby.example.com"
}
```

---

### 6.2 获取媒体统计

**接口**: `GET /user/media/stats`
**认证**: Bearer Token (User)
**描述**: 获取媒体库统计信息（缓存 5 分钟）

#### 请求示例

```bash
curl http://localhost:8080/api/v1/user/media/stats \
  -H "Authorization: Bearer {user_token}"
```

#### 响应示例

```json
{
  "success": true,
  "data": {
    "MovieCount": 1234,
    "SeriesCount": 567,
    "EpisodeCount": 8901
  }
}
```

---

## 7. 系统相关

### 7.1 获取系统信息

**接口**: `GET /admin/system/info`
**认证**: Bearer Token (Admin)
**描述**: 获取系统统计信息

#### 请求示例

```bash
curl http://localhost:8080/api/v1/admin/system/info \
  -H "Authorization: Bearer {admin_token}"
```

#### 响应示例

```json
{
  "success": true,
  "info": {
    "userCount": 100,
    "activeUserCount": 85,
    "inviteCount": 50
  }
}
```

---

### 7.2 测试 Emby 连接

**接口**: `POST /admin/system/test-emby`
**认证**: Bearer Token (Admin)
**描述**: 测试 Emby 服务器连接状态

#### 请求示例

```bash
curl -X POST http://localhost:8080/api/v1/admin/system/test-emby \
  -H "Authorization: Bearer {admin_token}"
```

#### 响应示例（成功）

```json
{
  "success": true,
  "message": "Emby 服务器连接正常"
}
```

#### 响应示例（失败）

```json
{
  "success": false,
  "error": "无法连接到 Emby 服务器：connection refused"
}
```

---

## 8. 定时任务

### 8.1 检查过期用户

**接口**: `POST /admin/cron/check-expired`
**认证**: Bearer Token (Admin)
**描述**: 检查并禁用过期用户（定时任务）

#### 请求示例

```bash
curl -X POST http://localhost:8080/api/v1/admin/cron/check-expired \
  -H "Authorization: Bearer {admin_token}"
```

#### 响应示例

```json
{
  "success": true,
  "disabledCount": 3,
  "totalExpired": 5,
  "errors": [
    "禁用用户 user1 失败: Emby API error"
  ]
}
```

**说明**:
- `disabledCount`: 成功禁用的用户数
- `totalExpired`: 总过期用户数
- `errors`: 失败的用户错误信息

---

## 9. 工具接口

### 9.1 TMDB 搜索

**接口**: `GET /tmdb/search`
**认证**: 无需认证
**描述**: 搜索电影或电视剧（TMDB API 代理）

#### 查询参数

| 参数名 | 类型 | 必填 | 默认值 | 说明 |
|--------|------|------|--------|------|
| query | string | 是 | - | 搜索关键词 |
| type | string | 否 | movie | 媒体类型（movie/tv） |

#### 请求示例

```bash
# 搜索电影
curl "http://localhost:8080/api/v1/tmdb/search?query=黑客帝国&type=movie"

# 搜索电视剧
curl "http://localhost:8080/api/v1/tmdb/search?query=权力的游戏&type=tv"
```

#### 响应示例

```json
{
  "results": [
    {
      "id": 603,
      "title": "黑客帝国",
      "originalTitle": "The Matrix",
      "overview": "程序员尼奥发现现实世界是一个由机器控制的虚拟世界...",
      "posterPath": "/f89U3ADr1oiB1s9GkdPOEpXUk5H.jpg",
      "releaseDate": "1999-03-31",
      "mediaType": "movie"
    }
  ],
  "total": 10
}
```

---

## 🔐 认证说明

### JWT Token 格式

所有需要认证的接口都需要在请求头中携带 JWT Token：

```
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

### Token 有效期

- **管理员 Token**: 7 天
- **用户 Token**: 7 天

### Token 刷新

目前不支持 Token 刷新，过期后需要重新登录。

---

## ⚠️ 错误响应格式

所有错误响应遵循统一格式：

```json
{
  "error": "错误描述信息"
}
```

### 常见 HTTP 状态码

| 状态码 | 说明 |
|--------|------|
| 200 | 成功 |
| 400 | 请求参数错误 |
| 401 | 未认证或 Token 无效 |
| 403 | 权限不足 |
| 404 | 资源不存在 |
| 500 | 服务器内部错误 |

---

## 🔧 环境变量

接口依赖以下环境变量：

```bash
# 数据库
DATABASE_URL=postgresql://user:pass@localhost:5432/ember

# JWT
JWT_SECRET=your-secret-key

# Emby
EMBY_URL=http://localhost:8096               # 内部地址
NEXT_PUBLIC_EMBY_URL=https://emby.example.com # 公网地址（优先）
EMBY_API_KEY=your-emby-api-key

# MoviePilot
MOVIEPILOT_URL=http://localhost:3000
MOVIEPILOT_USERNAME=admin
MOVIEPILOT_PASSWORD=password

# TMDB
TMDB_API_KEY=your-tmdb-api-key

# 服务器
PORT=8080
```

---

## 📝 版本历史

### v1.0 (2026-02-11)

- ✅ 初始版本
- ✅ 实现 33 个 REST API
- ✅ JWT 认证系统
- ✅ Emby 集成
- ✅ MoviePilot 集成
- ✅ TMDB 集成

---

**文档更新时间**: 2026-02-11
**API 版本**: v1.0
**联系方式**: 项目 GitHub Issues
