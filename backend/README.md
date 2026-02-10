# Ember Go Backend

> Emby 用户管理系统的 Go 语言后端实现

## 🎯 设计原则

### 与 Next.js 版本的兼容性

本 Go 后端遵循以下核心原则：

1. **数据兼容性** - 与 Prisma 创建的数据库表 100% 兼容
2. **业务语义保留** - 保留原有设计中的业务逻辑（如字符串外键）
3. **零停机迁移** - 可以与 Next.js API 并行运行
4. **渐进式替换** - 一次迁移一个 API 端点

### 关键设计决策

#### 1. 保留 `InviteCode string` 外键

```go
// ✅ 保留原设计
type User struct {
    InviteCode string `json:"inviteCode"` // 历史快照
    ...
}

// ❌ 不采用文档建议
// InviteID uuid.UUID // 错误：丢失业务语义
```

**理由：**
- 注册是"历史事件"（我用哪个码注册的），不是"持续关系"（我属于哪个邀请码）
- 类似电商订单保存商品名称，而不是仅保存商品ID
- 即使邀请码被删除，用户记录仍保留完整历史
- 详见：`prisma/schema.prisma` 第24-35行注释

#### 2. 使用 `cuid` 而非 `UUID`

```go
// ✅ 与 Prisma 保持一致
ID string `gorm:"type:varchar(25);primaryKey"` // cuid
```

**理由：**
- Prisma 使用 `@default(cuid())`
- 兼容现有数据库数据
- 对 < 100 用户规模，cuid vs UUID 性能差异可忽略

#### 3. 不执行 `AutoMigrate`

```go
// ⚠️ 重要：不执行 AutoMigrate
// 因为表已经由 Prisma 创建，我们只是连接现有数据库
```

**理由：**
- 避免破坏现有表结构
- 迁移由 Prisma 管理
- Go 后端只负责读写数据

## 🏗️ 项目结构

```
backend/
├── cmd/
│   └── server/
│       └── main.go           # 应用入口
├── internal/
│   ├── api/
│   │   ├── handlers/         # HTTP 处理器
│   │   └── middleware/       # 中间件
│   ├── models/               # GORM 数据模型
│   │   ├── admin.go
│   │   ├── invite.go
│   │   ├── user.go
│   │   └── subscription.go
│   ├── services/             # 业务逻辑层
│   └── db/
│       └── db.go             # 数据库初始化
├── migrations/               # 数据库迁移脚本（可选）
├── scripts/                  # 工具脚本
├── go.mod
└── README.md                 # 本文档
```

## 🚀 快速开始

### 1. 安装依赖

```bash
cd backend
go mod download
```

### 2. 配置环境变量

```bash
cp .env.example .env
# 编辑 .env，配置数据库连接
```

### 3. 运行服务

```bash
# 开发模式
go run cmd/server/main.go

# 或者构建后运行
go build -o bin/ember cmd/server/main.go
./bin/ember
```

### 4. 验证服务

```bash
# 健康检查
curl http://localhost:3001/health

# 预期响应
{
  "status": "ok",
  "message": "Ember Go Backend is running"
}
```

## 📊 当前状态

### ✅ 已完成

- [x] 项目结构搭建
- [x] GORM 数据模型（Admin, Invite, User, Subscription）
- [x] 数据库连接层
- [x] 健康检查 API
- [x] 设计决策文档

### 🚧 进行中

- [ ] 管理员认证 API
- [ ] 用户管理 CRUD
- [ ] 邀请码管理
- [ ] 订阅管理

### ⏳ 待开始

- [ ] JWT 中间件
- [ ] CORS 中间件
- [ ] 错误处理
- [ ] 日志记录
- [ ] 单元测试

## 🔄 与 Next.js 并行运行

### 架构图

```
浏览器
    │
    ├─→ Next.js Frontend (React)
    │       │
    │       ├─→ /api/admin/login  (Next.js API) ← 旧端点
    │       └─→ /api/v1/admin/login (Go Backend) ← 新端点
    │
    └─→ PostgreSQL 数据库 (共享)
```

### 迁移策略

1. **阶段1：并行运行**
   - Next.js API: `http://localhost:3000/api/*`
   - Go Backend: `http://localhost:3001/api/v1/*`

2. **阶段2：逐步替换**
   - 每迁移一个端点，前端切换到 Go 后端
   - 保留 Next.js 端点 1 周（以防回滚）

3. **阶段3：完全切换**
   - 所有 API 迁移完成后，停止 Next.js API

## 🎓 学习资源

### Go 语言基础

- [Go 官方教程](https://go.dev/tour/)
- [Effective Go](https://go.dev/doc/effective_go)

### Gin 框架

- [Gin 官方文档](https://gin-gonic.com/docs/)

### GORM

- [GORM 指南](https://gorm.io/docs/)
- [GORM 关联关系](https://gorm.io/docs/belongs_to.html)

## 📝 开发规范

### 代码风格

- 遵循 `gofmt` 格式
- 使用有意义的变量名
- 函数注释说明用途

### 错误处理

```go
// ✅ 正确
if err != nil {
    log.Printf("错误：%v", err)
    c.JSON(500, gin.H{"error": "内部错误"})
    return
}

// ❌ 错误
if err != nil {
    panic(err) // 永远不要 panic
}
```

## 🤝 贡献

欢迎提交 PR！请确保：

1. 代码通过 `go fmt` 格式化
2. 添加必要的注释
3. 测试通过

---

`★ Insight ─────────────────────────────────────`
**为什么保留 InviteCode string？**
- "Good taste" 是让数据结构反映业务现实
- 用户注册时记录的是"用了哪个码"（历史），不是"属于哪个码"（关系）
- UUID 外键虽然"理论正确"，但丢失了业务语义
- Linus: "Theory loses. Every single time."
`─────────────────────────────────────────────────`

Made with ❤️ by Kong Hang
