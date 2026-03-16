# Ember Go API 开发指南

> 快速开始指南和 API 测试说明

## 🚀 快速开始

### 1. 配置环境变量

```bash
# 复制环境变量文件
cp .env.example .env

# 编辑 .env，至少需要设置：
# - DATABASE_URL（数据库连接）
# - JWT_SECRET（至少 32 字符）
```

### 2. 启动服务

#### 方式 A：使用 Makefile（推荐）

```bash
cd ../../  # 回到项目根目录
make dev-api
```

#### 方式 B：直接运行

```bash
go run cmd/server/main.go
```

#### 方式 C：编译后运行

```bash
go build -o bin/ember cmd/server/main.go
./bin/ember
```

### 3. 验证服务

```bash
# 健康检查
curl http://localhost:8080/health

# 预期响应
{
  "status": "ok",
  "message": "Ember Go API is running"
}
```

---

## 📋 已实现的 API（第 1 优先级）

### ✅ 认证相关

#### 管理员登录
```bash
POST /api/v1/admin/login
Content-Type: application/json

{
  "username": "admin",
  "password": "admin123"
}

# 响应
{
  "token": "eyJhbGciOiJI...",
  "admin": {
    "id": "cl...",
    "username": "admin",
    "createdAt": "2024-01-01T00:00:00Z"
  }
}
```

#### 获取当前用户
```bash
GET /api/v1/admin/current
Authorization: Bearer {token}

# 响应
{
  "user": {
    "id": "cl...",
    "username": "admin",
    ...
  }
}
```

#### 登出
```bash
POST /api/v1/admin/logout
Authorization: Bearer {token}

# 响应
{
  "message": "登出成功"
}
```

---

### ✅ 用户管理

#### 获取用户列表
```bash
GET /api/v1/admin/users?page=1&pageSize=20&search=username&expiresAfter=2026-03-01&embyStatus=disabled
Authorization: Bearer {token}

# 响应
{
  "data": [...],
  "total": 100,
  "page": 1,
  "pageSize": 20,
  "totalPages": 5
}
```

`expiresAfter` 为可选参数，格式 `YYYY-MM-DD`，用于筛选“到期时间晚于该日期”的用户（不包含永不过期用户）。
`embyStatus` 为可选参数，支持：`available`（可用）、`disabled`（禁用）、`unlinked`（未关联）。

#### 获取用户详情
```bash
GET /api/v1/admin/users/{id}
Authorization: Bearer {token}
```

#### 延长到期时间
```bash
PUT /api/v1/admin/users/{id}/extend
Authorization: Bearer {token}
Content-Type: application/json

{
  "days": 30
}
```

#### 启用/禁用用户
```bash
PUT /api/v1/admin/users/{id}/toggle
Authorization: Bearer {token}
```

#### 重置密码
```bash
PUT /api/v1/admin/users/{id}/reset-password
Authorization: Bearer {token}
Content-Type: application/json

{
  "newPassword": "newpass123"
}
```

#### 删除用户
```bash
DELETE /api/v1/admin/users/{id}
Authorization: Bearer {token}
```

---

### ✅ 邀请码管理

#### 获取邀请码列表
```bash
GET /api/v1/admin/invites?page=1&pageSize=20&showAll=false
Authorization: Bearer {token}

# showAll=false 只显示有效的邀请码
# showAll=true 显示所有邀请码（包括已用完和已过期）
```

#### 创建邀请码
```bash
POST /api/v1/admin/invites
Authorization: Bearer {token}
Content-Type: application/json

{
  "maxUses": 5,
  "defaultDays": 30,
  "expiresAt": "2024-12-31T23:59:59Z"  // 可选
}

# 响应
{
  "id": "cl...",
  "code": "a1b2c3d4e5f6g7h8",  // 自动生成的随机码
  "maxUses": 5,
  "usedCount": 0,
  "defaultDays": 30,
  "createdAt": "2024-01-01T00:00:00Z"
}
```

#### 删除邀请码
```bash
DELETE /api/v1/admin/invites/{id}
Authorization: Bearer {token}
```

#### 验证邀请码（公开接口）
```bash
GET /api/v1/invites/{code}/validate

# 响应（有效）
{
  "id": "cl...",
  "code": "a1b2c3d4e5f6g7h8",
  "maxUses": 5,
  "usedCount": 2,
  "defaultDays": 30
}

# 响应（无效）
{
  "error": "邀请码已失效"
}
```

---

## 🧪 自动化测试

使用提供的测试脚本：

```bash
# 确保 API 服务已启动
go run cmd/server/main.go

# 在另一个终端运行测试
./test_api.sh
```

测试脚本会依次测试：
1. 健康检查
2. 管理员登录
3. 获取当前用户
4. 获取用户列表
5. 获取邀请码列表
6. 创建邀请码

---

## 📊 进度统计

### ✅ 已完成（11/33）

| 分类 | 已实现 | 总计 | 进度 |
|------|--------|------|------|
| 认证相关 | 5/7 | 7 | 71% |
| 用户管理 | 6/6 | 6 | 100% |
| 邀请码管理 | 4/4 | 4 | 100% |
| 订阅管理 | 0/6 | 6 | 0% |
| 用户面板 | 0/4 | 4 | 0% |
| 其他 | 0/6 | 6 | 0% |
| **总计** | **15/33** | **33** | **45%** |

### 🚧 下一步（第 2-3 优先级）

- [ ] 用户注册 API（使用邀请码）
- [ ] 用户登录 API（Emby 验证）
- [ ] 订阅管理 API（6 个）
- [ ] 用户面板 API（4 个）

---

## 🔧 开发技巧

### JWT Token 获取

```bash
# 登录并提取 Token
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/admin/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}' | jq -r '.token')

# 使用 Token 访问受保护的 API
curl http://localhost:8080/api/v1/admin/users \
  -H "Authorization: Bearer $TOKEN"
```

### 查看结构化日志

```bash
# 开发模式（详细日志）
GIN_MODE=debug go run cmd/server/main.go

# 生产模式（简洁日志）
GIN_MODE=release go run cmd/server/main.go
```

### 热重载开发

安装 air：
```bash
go install github.com/air-verse/air@latest
```

使用 air 启动：
```bash
air
```

---

## 🐛 常见问题

### 1. JWT_SECRET 未设置

```text
错误：❌ JWT 初始化失败：JWT_SECRET 环境变量未设置
解决：在 .env 文件中设置 JWT_SECRET（至少 32 字符）
```

### 2. 数据库连接失败

```text
错误：❌ 无法连接数据库
解决：
1. 检查 PostgreSQL 是否启动
2. 检查 .env 中的 DATABASE_URL 是否正确
3. 确保数据库 ember 已创建
```

### 3. 登录失败

```text
错误：{"error":"用户名或密码错误"}
解决：
1. 确保数据库中有管理员账号
2. 可以先运行 Next.js 项目创建管理员
3. 或者手动插入数据库（密码使用 bcrypt 加密）
```

---

## 📚 相关文档

- [项目 README](../../README.md)
- [文档中心](../../docs/README.md)
- [Makefile 命令](../../Makefile)

---

Made with ❤️ by Kong Hang
