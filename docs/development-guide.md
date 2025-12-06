# Ember 开发指南

> 本文档提供开发环境搭建、代码规范和开发流程说明

---

## 🚀 环境搭建

### 1. 前置要求

- **Node.js**: v18.17+ 或 v20.x
- **npm**: v9.x+
- **PostgreSQL**: v16.x
- **Git**: v2.x+

### 2. 克隆项目

```bash
git clone https://github.com/yourusername/ember.git
cd ember
```

### 3. 安装依赖

```bash
npm install
```

### 4. 配置环境变量

```bash
# 复制环境变量模板
cp .env.example .env

# 编辑 .env 文件，配置以下必需项：
# - DATABASE_URL: PostgreSQL 数据库连接
# - JWT_SECRET: JWT 密钥（至少 32 个字符）
# - EMBY_URL: Emby 服务器地址
# - EMBY_API_KEY: Emby API 密钥
```

### 5. 初始化数据库

```bash
# 生成 Prisma Client
npx prisma generate

# 运行数据库迁移
npx prisma migrate dev

# （可选）查看数据库
npx prisma studio
```

### 6. 启动开发服务器

```bash
npm run dev
```

访问 http://localhost:3000

---

## 📁 项目结构

```
ember/
├── app/                    # Next.js App Router
│   ├── (auth)/            # 认证页面组
│   │   ├── login/         # 管理员登录
│   │   └── register/      # 用户注册
│   ├── (admin)/           # 管理后台
│   │   ├── invites/       # 邀请码管理
│   │   ├── users/         # 用户管理
│   │   ├── settings/      # 系统设置
│   │   └── layout.tsx     # 后台布局（含导航和路由保护）
│   ├── actions/           # Server Actions
│   │   ├── admin.ts       # 管理员相关
│   │   ├── invites.ts     # 邀请码相关
│   │   ├── users.ts       # 用户相关
│   │   ├── cron.ts        # 定时任务
│   │   └── settings.ts    # 系统设置
│   └── api/               # API Routes
│       └── cron/          # Cron 定时任务端点
│
├── lib/                   # 工具库
│   ├── db.ts             # Prisma Client
│   ├── auth.ts           # JWT 认证工具
│   ├── emby.ts           # Emby API 客户端
│   └── utils.ts          # 通用工具函数
│
├── prisma/               # Prisma 配置
│   ├── schema.prisma     # 数据库 Schema
│   └── migrations/       # 迁移文件
│
├── docs/                 # 项目文档
│   ├── specs/           # 需求和设计文档
│   └── *.md             # 各类文档
│
├── .env                  # 环境变量（不提交到 Git）
├── .env.example          # 环境变量模板
├── vercel.json          # Vercel Cron 配置
└── package.json         # 项目依赖
```

---

## 📝 代码规范

### 1. 文件命名

- **组件文件**: PascalCase，如 `UserList.tsx`
- **工具文件**: camelCase，如 `auth.ts`
- **配置文件**: kebab-case，如 `next.config.js`

### 2. TypeScript 规范

- **严格模式**: 启用 TypeScript strict mode
- **类型定义**: 优先使用 interface，复杂类型用 type
- **避免 any**: 除非必要，否则不使用 any

### 3. Server Actions 规范

所有 Server Actions 必须：

```typescript
'use server'

import { prisma } from '@/lib/db'

export async function actionName(params: Params) {
  try {
    // 1. 验证参数
    // 2. 业务逻辑
    // 3. 数据库操作
    // 4. 记录日志

    return {
      success: true,
      data: result,
    }
  } catch (error) {
    console.error('操作失败:', error)
    return {
      success: false,
      error: '错误信息',
    }
  }
}
```

### 4. 数据库操作规范

- **使用事务**: 涉及多个表操作时使用 `prisma.$transaction`
- **错误处理**: 数据库操作必须 try-catch
- **记录日志**: 重要操作记录到 Log 表

示例：

```typescript
await prisma.$transaction(async (tx) => {
  // 操作 1
  await tx.user.create({ ... })

  // 操作 2
  await tx.log.create({ ... })
})
```

---

## 🔐 认证和授权

### JWT 认证流程

1. **管理员登录** (`app/actions/admin.ts`):
   - 验证用户名和密码
   - 生成 JWT token (7天有效)
   - 返回 token 给前端

2. **Token 存储** (前端):
   - 存储到 `localStorage`
   - 每次请求自动携带

3. **路由保护** (`app/(admin)/layout.tsx`):
   - 读取 localStorage 中的 token
   - 验证 token 有效性
   - 无效则跳转登录页

### JWT 工具函数

```typescript
import { generateToken, verifyToken } from '@/lib/auth'

// 生成 token
const token = generateToken({ username: 'admin' })

// 验证 token
const payload = verifyToken(token) // { username: 'admin' }
```

---

## 🔄 定时任务

### Vercel Cron 配置

在 `vercel.json` 中配置：

```json
{
  "crons": [
    {
      "path": "/api/cron",
      "schedule": "0 2 * * *"
    }
  ]
}
```

### 本地测试定时任务

```bash
# 方法 1: 通过系统设置页面手动触发
访问 http://localhost:3000/admin/settings，点击"手动触发"按钮

# 方法 2: 直接调用 API
curl http://localhost:3000/api/cron
```

---

## 🧪 测试

### 运行构建

```bash
npm run build
```

### 类型检查

```bash
npx tsc --noEmit
```

---

## 📦 部署

### Vercel 部署

1. 推送代码到 GitHub
2. 在 Vercel 导入项目
3. 配置环境变量
4. 自动部署

### Docker 部署

```bash
# 构建镜像
docker build -t ember .

# 运行容器
docker run -p 3000:3000 --env-file .env ember
```

---

## 🐛 调试技巧

### 1. 查看 Server Actions 日志

Server Actions 的日志会输出到终端：

```bash
npm run dev
# 查看终端输出
```

### 2. 查看数据库

```bash
npx prisma studio
```

### 3. 查看 API 请求

使用浏览器开发者工具的 Network 面板

---

## 📚 常见问题

### Q: Prisma Client 找不到？

```bash
npx prisma generate
```

### Q: 数据库连接失败？

检查 `.env` 中的 `DATABASE_URL` 是否正确

### Q: JWT token 无效？

确保 `JWT_SECRET` 在前后端一致，且至少 32 个字符

### Q: Emby API 调用失败？

1. 检查 `EMBY_URL` 和 `EMBY_API_KEY` 是否正确
2. 在系统设置页面测试 Emby 连接

---

## 🤝 贡献指南

### 提交代码流程

1. **Fork 项目**
2. **创建功能分支**:
   ```bash
   git checkout -b feature/amazing-feature
   ```
3. **提交更改**:
   ```bash
   git commit -m 'feat: 添加某功能'
   ```
4. **推送到分支**:
   ```bash
   git push origin feature/amazing-feature
   ```
5. **创建 Pull Request**

### Commit 规范

使用 [Conventional Commits](https://www.conventionalcommits.org/):

- `feat`: 新功能
- `fix`: Bug 修复
- `docs`: 文档更新
- `style`: 代码格式
- `refactor`: 重构
- `test`: 测试相关
- `chore`: 构建/工具相关

示例：
```
feat: 添加用户搜索功能
fix: 修复邀请码过期判断错误
docs: 更新开发指南
```

---

## 📞 获取帮助

- **文档**: [docs/](../docs/)
- **Issues**: [GitHub Issues](https://github.com/yourusername/ember/issues)
- **讨论**: [GitHub Discussions](https://github.com/yourusername/ember/discussions)

---

**最后更新**: 2025-12-06
