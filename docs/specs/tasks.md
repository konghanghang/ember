# Ember MVP - 核心用户管理系统 - Task List

## 📅 Timeline Overview

```
Week 1 (Day 1-5)  → 基础设施 + 核心功能
Week 2 (Day 6-10) → 到期管理 + 部署 + 测试
```

**总开发时间**：10 个工作日（2 周）

---

## Week 1: 基础设施 + 核心功能

### Day 1-2: 项目初始化 + 数据库 + 管理员登录

- [x] **1. 项目初始化**
    - [x] 1.1. 创建 Next.js 15 项目
        - *Goal*: 初始化项目脚手架
        - *Details*:
          ```bash
          npx create-next-app@latest ember --typescript --tailwind --app
          cd ember
          ```
        - *配置*:
          - TypeScript 严格模式
          - App Router
          - Tailwind CSS
        - *时间*: 1 小时

    - [x] 1.2. 配置 Prisma + PostgreSQL
        - *Goal*: 设置数据库 ORM
        - *Details*:
          ```bash
          npm install prisma @prisma/client bcryptjs jsonwebtoken date-fns
          npm install -D @types/bcryptjs @types/jsonwebtoken
          npx prisma init
          ```
        - *配置*:
          - 创建 `prisma/schema.prisma`（4 张表）
          - 配置 `.env` 中的 `DATABASE_URL`
          - 创建 `lib/db.ts`（Prisma 客户端单例）
        - *时间*: 2 小时

    - [x] 1.3. 数据库 Schema 实现
        - *Goal*: 创建数据库表
        - *Details*:
          - 实现 `Admin`、`Invite`、`User`、`Log` 四个 model
          - 添加索引：`username`、`embyId`、`expiresAt`、`code`
          - 配置关系：`Invite` → `User`（1对多）
        - *验证*:
          ```bash
          npx prisma migrate dev --name init
          npx prisma generate
          ```
        - *时间*: 2 小时

    - [x] 1.4. 创建初始管理员（Seed）
        - *Goal*: 初始化默认管理员账号
        - *Details*:
          - 创建 `prisma/seed.ts`
          - 默认账号：`admin` / `admin123`
          - 密码使用 bcrypt 加密（cost=10）
        - *验证*:
          ```bash
          npx prisma db seed
          ```
        - *时间*: 1 小时

- [x] **2. 管理员登录功能**
    - [x] 2.1. JWT 工具函数
        - *Goal*: 实现 JWT Token 生成和验证
        - *Details*:
          - 创建 `lib/auth.ts`
          - 函数：`signToken()`、`verifyToken()`、`hashPassword()`、`verifyPassword()`
          - Token 有效期：7 天
          - JWT_SECRET 从环境变量读取
        - *时间*: 2 小时

    - [x] 2.2. 登录 Server Action
        - *Goal*: 实现管理员登录逻辑
        - *Details*:
          - 创建 `app/actions/auth.ts`
          - 实现 `adminLogin()` 函数
          - 验证用户名和密码
          - 返回 JWT Token
          - 记录登录日志
        - *时间*: 2 小时

    - [x] 2.3. 登录页面 UI
        - *Goal*: 创建管理员登录界面
        - *Details*:
          - 创建 `app/(auth)/login/page.tsx`
          - 使用 shadcn/ui 表单组件
          - Token 存储到 localStorage
          - 登录成功跳转到 `/admin/users`
        - *时间*: 3 小时

    - [x] 2.4. 路由保护中间件
        - *Goal*: 保护管理后台路由
        - *Details*:
          - 创建 `app/(admin)/layout.tsx`
          - 验证 localStorage 中的 Token
          - 无效/过期 Token 跳转到 `/login`
        - *时间*: 2 小时

**Day 1-2 总时间**: 15 小时

---

### Day 3-4: 邀请码生成 + 用户注册 + Emby 集成

- [x] **3. Emby API 客户端**
    - [x] 3.1. 实现 Emby API 封装
        - *Goal*: 创建 Emby API 客户端
        - *Details*:
          - 创建 `lib/emby.ts`
          - 实现 `EmbyClient` 类
          - 方法：
            - `createUser()` - 创建 Emby 用户
            - `deleteUser()` - 删除 Emby 用户
            - `setUserPolicy()` - 更新用户权限（禁用/启用）
            - `getUsers()` - 获取用户列表
          - 重试机制：失败重试 3 次，指数退避
          - 环境变量：`EMBY_URL`、`EMBY_API_KEY`
        - *时间*: 4 小时

    - [ ] 3.2. 测试 Emby API 连接
        - *Goal*: 验证 Emby API 可用
        - *Details*:
          - 创建测试脚本 `scripts/test-emby.ts`
          - 测试创建/删除用户
          - 验证权限设置
        - *时间*: 1 小时

- [x] **4. 邀请码管理**
    - [x] 4.1. 邀请码生成工具
        - *Goal*: 实现邀请码生成逻辑
        - *Details*:
          - 在 `lib/utils.ts` 中实现 `generateInviteCode()`
          - 8 位随机字符串（大小写字母 + 数字）
          - 确保唯一性（查询数据库）
        - *时间*: 1 小时

    - [x] 4.2. 邀请码 Server Actions
        - *Goal*: 实现邀请码 CRUD
        - *Details*:
          - 创建 `app/actions/invites.ts`
          - 函数：
            - `createInvite()` - 生成邀请码
            - `getInvites()` - 获取邀请码列表（含使用情况）
            - `deleteInvite()` - 删除邀请码（检查是否已使用）
            - `validateInvite()` - 验证邀请码有效性
        - *时间*: 3 小时

    - [x] 4.3. 邀请码管理页面
        - *Goal*: 创建邀请码管理界面
        - *Details*:
          - 创建 `app/(admin)/invites/page.tsx`
          - 邀请码列表表格（code、使用次数、过期时间）
          - 生成邀请码表单（最大使用次数、过期时间、默认天数）
          - 删除邀请码按钮
        - *时间*: 4 小时

- [x] **5. 用户注册功能**
    - [x] 5.1. 用户注册 Server Action
        - *Goal*: 实现用户注册逻辑（核心功能）
        - *Details*:
          - 创建 `app/actions/users.ts`
          - 实现 `registerUser()` 函数
          - 流程：
            1. 验证邀请码有效性
            2. 使用 Prisma `$transaction`：
               - 调用 Emby API 创建用户
               - 保存用户到数据库
               - 更新邀请码使用次数
               - 记录日志
            3. 失败时自动回滚
          - 错误处理：
            - Emby API 失败 → 事务回滚
            - 用户名已存在 → 抛出错误
        - *时间*: 4 小时

    - [x] 5.2. 用户注册页面
        - *Goal*: 创建用户注册界面
        - *Details*:
          - 创建 `app/(auth)/register/page.tsx`
          - 表单字段：邀请码、用户名、密码、邮箱
          - 显示邀请码配置信息（默认天数）
          - 注册成功后显示 Emby 服务器地址
        - *时间*: 3 小时

**Day 3-4 总时间**: 20 小时

---

### Day 5: 用户管理 + 延长到期

- [x] **6. 用户列表和管理**
    - [x] 6.1. 用户查询 Server Actions
        - *Goal*: 实现用户列表查询
        - *Details*:
          - 在 `app/actions/users.ts` 中实现：
            - `getUsers()` - 获取用户列表（支持搜索）
            - `extendExpiry()` - 延长到期时间
          - 搜索逻辑：按用户名模糊查询（case insensitive）
          - Include 邀请码信息
        - *时间*: 2 小时

    - [x] 6.2. 用户列表页面
        - *Goal*: 创建用户管理界面
        - *Details*:
          - 创建 `app/(admin)/users/page.tsx`
          - 用户列表表格：
            - 用户名、邮箱、创建时间
            - 到期时间（显示剩余天数，颜色标识）
            - 账号状态（正常/已禁用）
            - 操作按钮：延长到期、禁用/启用、删除
          - 搜索框（实时搜索）
        - *时间*: 5 小时

    - [x] 6.3. 延长到期功能
        - *Goal*: 实现延长到期时间
        - *Details*:
          - 弹窗输入延长天数
          - 调用 `extendExpiry()` Server Action
          - 成功后刷新列表
          - 显示新的到期时间
        - *时间*: 2 小时

**Day 5 总时间**: 9 小时

---

## Week 2: 到期管理 + 部署 + 测试

### Day 6-7: 禁用/删除用户 + 定时任务

- [x] **7. 用户禁用和删除**
    - [x] 7.1. 禁用/启用 Server Action
        - *Goal*: 实现用户禁用/启用
        - *Details*:
          - 在 `app/actions/users.ts` 中实现 `toggleUserStatus()`
          - 同步到 Emby：调用 `setUserPolicy()`
          - 更新数据库 `isActive` 字段
          - 记录日志
        - *时间*: 2 小时

    - [x] 7.2. 删除用户 Server Action
        - *Goal*: 实现用户删除
        - *Details*:
          - 在 `app/actions/users.ts` 中实现 `deleteUser()`
          - 使用 `$transaction`：
            1. 删除 Emby 用户
            2. 删除数据库记录
            3. 记录日志（保存用户名和 embyId）
          - 错误处理：Emby API 失败时回滚
        - *时间*: 2 小时

    - [x] 7.3. UI 集成
        - *Goal*: 在用户列表中添加禁用/删除功能
        - *Details*:
          - 禁用/启用按钮（切换状态）
          - 删除按钮（确认对话框）
          - 操作成功后刷新列表
        - *时间*: 2 小时

- [x] **8. 定时任务：到期检查**
    - [x] 8.1. 定时任务 Server Action
        - *Goal*: 实现到期账号检查逻辑
        - *Details*:
          - 创建 `app/actions/cron.ts`
          - 实现 `checkExpiredUsers()` 函数
          - 查询已到期但仍启用的用户
          - 循环处理：
            1. 调用 Emby API 禁用
            2. 更新数据库状态
            3. 记录日志
          - 错误处理：单个用户失败不影响其他用户
        - *时间*: 3 小时

    - [x] 8.2. Vercel Cron API 路由
        - *Goal*: 设置定时任务调度
        - *Details*:
          - 创建 `app/api/cron/route.ts`
          - 创建 `vercel.json` 配置文件
          - 定时规则：每天凌晨 2:00
          - 调用 `checkExpiredUsers()`
          - 记录执行结果（禁用用户数）
        - *时间*: 2 小时

    - [x] 8.3. 手动触发到期检查
        - *Goal*: 在管理后台添加手动触发按钮
        - *Details*:
          - 在设置页面 `app/(admin)/settings/page.tsx` 添加按钮
          - 点击后调用 `checkExpiredUsers()`
          - 显示执行结果（禁用了 X 个用户）
        - *时间*: 2 hours

**Day 6-7 总时间**: 13 小时

---

### Day 8: Docker 部署 + 配置管理

- [x] **9. 系统配置管理**
    - [x] 9.1. 系统信息 Server Actions
        - *Goal*: 实现系统信息查询
        - *Details*:
          - 创建 `app/actions/settings.ts`
          - 实现 `getSystemInfo()` - 获取用户数、邀请码数等
          - 实现 `testEmbyConnection()` - 测试 Emby 连接
        - *时间*: 1 小时

    - [x] 9.2. 系统设置页面
        - *Goal*: 创建设置界面
        - *Details*:
          - 创建 `app/(admin)/settings/page.tsx`
          - 显示系统信息（用户数、活跃用户数、邀请码数）
          - 测试 Emby 连接按钮
          - 手动触发到期检查按钮
          - 显示环境信息
        - *时间*: 3 小时

- [ ] **10. Docker 部署配置**
    - [ ] 10.1. Dockerfile
        - *Goal*: 创建 Docker 镜像
        - *Details*:
          - 多阶段构建（deps → builder → runner）
          - 复制 Prisma 文件和生成的客户端
          - 暴露端口 3000
          - 配置 Next.js standalone 输出
        - *时间*: 2 小时

    - [ ] 10.2. docker-compose.yml
        - *Goal*: 配置服务编排
        - *Details*:
          - 服务：ember + postgres
          - 数据卷：postgres_data（持久化）
          - 环境变量配置
          - 依赖关系：ember depends_on postgres
        - *时间*: 1 小时

    - [ ] 10.3. 部署测试
        - *Goal*: 验证 Docker 部署
        - *Details*:
          ```bash
          docker compose build
          docker compose up -d
          docker compose exec ember npx prisma migrate deploy
          docker compose exec ember npx prisma db seed
          ```
          - 访问 http://localhost:3000
          - 测试完整流程
        - *时间*: 2 小时

**Day 8 总时间**: 9 小时

---

### Day 9-10: 测试 + Bug 修复 + 文档

- [ ] **11. 功能测试**
    - [ ] 11.1. 管理员功能测试
        - *Goal*: 验证管理员所有功能
        - *Details*:
          - [ ] 登录（正确/错误密码）
          - [ ] Token 过期自动跳转
          - [ ] 生成邀请码（验证唯一性）
          - [ ] 删除邀请码（已使用/未使用）
        - *时间*: 2 小时

    - [ ] 11.2. 用户注册测试
        - *Goal*: 验证用户注册流程
        - *Details*:
          - [ ] 有效邀请码注册成功
          - [ ] 无效邀请码注册失败
          - [ ] 用户名重复注册失败
          - [ ] Emby 用户创建成功
          - [ ] 数据库记录正确
          - [ ] 邀请码使用次数更新
        - *时间*: 2 小时

    - [ ] 11.3. 用户管理测试
        - *Goal*: 验证用户管理功能
        - *Details*:
          - [ ] 用户列表显示正确
          - [ ] 搜索功能正常
          - [ ] 延长到期时间成功
          - [ ] 禁用用户（Emby 同步）
          - [ ] 启用用户（Emby 同步）
          - [ ] 删除用户（Emby + 数据库）
        - *时间*: 3 小时

    - [ ] 11.4. 定时任务测试
        - *Goal*: 验证到期检查功能
        - *Details*:
          - [ ] 手动创建已到期用户
          - [ ] 手动触发到期检查
          - [ ] 验证用户被禁用
          - [ ] 验证日志记录
          - [ ] 验证 Emby 用户状态
        - *时间*: 2 小时

- [ ] **12. Bug 修复和优化**
    - [ ] 12.1. 错误处理优化
        - *Goal*: 完善错误提示
        - *Details*:
          - 统一错误格式
          - 友好的错误提示
          - 表单验证优化
        - *时间*: 2 小时

    - [ ] 12.2. UI/UX 优化
        - *Goal*: 改进用户体验
        - *Details*:
          - Loading 状态
          - 成功/失败 Toast 提示
          - 表格排序
          - 响应式布局优化
        - *时间*: 3 小时

- [ ] **13. 文档完善**
    - [ ] 13.1. README 更新
        - *Goal*: 更新项目文档
        - *Details*:
          - 快速开始指南
          - 环境变量说明
          - Docker 部署步骤
          - 常见问题
        - *时间*: 2 小时

    - [ ] 13.2. 部署文档
        - *Goal*: 编写部署指南
        - *Details*:
          - Docker 部署详细步骤
          - 环境变量配置
          - 数据库迁移
          - 初始管理员创建
          - 备份建议
        - *时间*: 2 小时

**Day 9-10 总时间**: 18 小时

---

## Task Dependencies

### 串行依赖（必须按顺序）

1. **项目初始化** → 所有其他任务
   - 必须先创建项目和数据库

2. **数据库 Schema** → 所有 Server Actions
   - 必须先有数据库表才能操作数据

3. **Emby API 客户端** → 用户注册、禁用/删除
   - 必须先实现 Emby 集成

4. **管理员登录** → 所有管理后台功能
   - 必须先能登录才能访问后台

5. **用户注册** → 用户管理
   - 必须先有用户才能管理

### 并行任务（可同时进行）

- **邀请码管理** + **Emby API 客户端**（Day 3）
  - 两者独立，可同时开发

- **用户列表** + **定时任务**（可分人开发）
  - 功能独立

- **Docker 配置** + **测试脚本编写**
  - 可并行准备

---

## Estimated Timeline

### Week 1 (44 hours)
- Day 1-2: 项目初始化 + 管理员登录 → **15 小时**
- Day 3-4: 邀请码 + 用户注册 + Emby 集成 → **20 小时**
- Day 5: 用户管理 + 延长到期 → **9 小时**

### Week 2 (40 hours)
- Day 6-7: 禁用/删除 + 定时任务 → **13 小时**
- Day 8: Docker + 配置 → **9 小时**
- Day 9-10: 测试 + 修复 + 文档 → **18 小时**

### 总计
- **开发时间**: 84 小时
- **工作日**: 10 天（2 周）
- **平均每天**: 8-9 小时

---

## Risk Mitigation

### 风险 1: Emby API 不稳定
- **缓解**: Day 3 完成 Emby 客户端后立即测试
- **预留**: 多预留 2-3 小时处理 Emby API 问题

### 风险 2: 时间估算偏差
- **缓解**: 每个任务预留 20% buffer
- **调整**: 如果 Day 5 完成不了，砍掉搜索功能（手动筛选）

### 风险 3: Docker 部署问题
- **缓解**: Day 8 专门用于部署测试
- **备选**: 如果 Docker 有问题，先用开发模式部署

---

## Success Criteria

MVP 完成的标准：

- [ ] **功能完整性**
  - [ ] 管理员能登录
  - [ ] 能生成和使用邀请码
  - [ ] 用户能注册（Emby 用户创建成功）
  - [ ] 能查看和管理用户列表
  - [ ] 能延长用户到期时间
  - [ ] 能禁用/删除用户（Emby 同步）
  - [ ] 定时任务能自动禁用过期用户

- [ ] **代码质量**
  - [ ] TypeScript 无编译错误
  - [ ] 所有 Server Actions 有错误处理
  - [ ] Emby API 调用有重试机制
  - [ ] 事务回滚机制正常工作

- [ ] **部署能力**
  - [ ] `docker compose up` 能一键启动
  - [ ] 数据持久化正常（PostgreSQL 数据卷）
  - [ ] 环境变量配置正确

- [ ] **测试通过**
  - [ ] 所有核心功能手动测试通过
  - [ ] 完整用户流程（注册→使用→到期→禁用）能走通

---

## Notes

### 开发建议

1. **数据结构优先**
   - Day 1 把数据库 Schema 设计好
   - 后续开发都基于这个结构

2. **先做 Emby 集成**
   - Day 3 优先实现 Emby API 客户端
   - 避免后期发现 API 不可用

3. **增量测试**
   - 每完成一个功能立即测试
   - 不要等到 Day 9 才开始测试

4. **简化 UI**
   - MVP 阶段不追求 UI 完美
   - 功能可用 > 界面美观

### 可选任务（时间充裕时）

- [ ] 仪表盘页面（用户统计）
- [ ] 用户详情页面（查看观看记录）
- [ ] 邀请码过滤和排序
- [ ] 用户列表分页

---

## 🔒 安全增强计划（Future Enhancements）

> **背景**：当前邀请码使用8位随机字符串，虽然搜索空间足够大（62^8 ≈ 218万亿），但缺少速率限制和审计机制，存在被暴力扫描的理论风险。

### 优先级排序

按照"最小代价，最大收益"原则实施：

#### **优先级 1：邀请码验证速率限制** ⭐⭐⭐

- [ ] **实现邀请码验证速率限制**
  - *Goal*: 防止暴力扫描邀请码
  - *Details*:
    - 在 `validateInvite()` 中添加速率限制
    - 每个邀请码验证失败 10 次后锁定 1 小时
    - 技术选型：Upstash Redis / Vercel KV / 内存缓存
    - 错误响应：`429 Too Many Requests`
  - *受影响文件*:
    - `app/actions/invites.ts` (validateInvite 函数)
    - 新建 `lib/ratelimit.ts` (速率限制工具)
  - *预计时间*: 30 分钟
  - *收益*: 直接阻断暴力扫描攻击

#### **优先级 2：邀请码验证失败审计日志** ⭐⭐⭐

- [ ] **添加邀请码验证失败的审计日志**
  - *Goal*: 记录所有失败的验证尝试，便于发现异常行为
  - *Details*:
    - 在 `validateInvite()` 失败时记录日志
    - 日志内容：尝试的邀请码、失败原因、IP 地址（如果可获取）、时间戳
    - 利用现有 `Log` 表，新增 action 类型：`validate_invite_failed`
  - *受影响文件*:
    - `app/actions/invites.ts` (validateInvite 函数)
  - *预计时间*: 15 分钟
  - *收益*: 可发现扫描行为，数据驱动决策

#### **优先级 3：注册接口速率限制** ⭐⭐

- [ ] **为注册接口添加速率限制**
  - *Goal*: 防止恶意批量注册
  - *Details*:
    - 限制每 IP 每小时最多 3 次注册尝试
    - 注册失败后计数，成功后重置
    - 需要获取客户端 IP（`x-forwarded-for` 或 `x-real-ip`）
  - *受影响文件*:
    - `app/actions/users.ts` (registerUser 函数)
    - `lib/ratelimit.ts` (新增 IP 维度限制)
  - *预计时间*: 20 分钟
  - *收益*: 防止资源滥用

#### **优先级 4：扫描行为监控和报警** ⭐

- [ ] **添加扫描行为监控和报警机制**
  - *Goal*: 主动发现异常行为并通知管理员
  - *Details*:
    - 检测短时间内大量验证失败（如：1 分钟内失败 20 次）
    - 报警渠道：Telegram Bot / 邮件 / Webhook
    - 可选：集成到现有监控系统（如 Sentry）
  - *受影响文件*:
    - 新建 `lib/monitoring.ts` (监控逻辑)
    - `app/actions/invites.ts` (触发监控)
  - *预计时间*: 按需（需要外部服务配置）
  - *收益*: 主动防御，及时响应

#### **可选：邀请码格式迁移到 UUID**

- [ ] **将邀请码从 8 位字符串迁移到 UUID**
  - *Goal*: 提升理论安全性，消除碰撞检测逻辑
  - *Details*:
    - 使用 `crypto.randomUUID()` 生成邀请码
    - 删除 `createInvite()` 中的 10 次碰撞检测循环
    - 数据库 schema 无需修改（已经是 String 类型）
    - 向后兼容：新旧格式共存
  - *受影响文件*:
    - `lib/utils.ts` (generateInviteCode 函数)
    - `app/actions/invites.ts` (删除碰撞检测)
  - *预计时间*: 10 分钟
  - *副作用*:
    - ✅ 代码更简洁（删除碰撞检测循环）
    - ❌ 日志可读性下降（36 字符 vs 8 字符）
    - ❌ UI 展示占用更多空间
  - *建议*: 仅在完成优先级 1-3 后，确认有实际需求时再实施

---

### 技术选型建议

#### **速率限制实现方案**

**推荐：Upstash Redis**
- ✅ Serverless 友好，按需计费
- ✅ 官方提供 `@upstash/ratelimit` SDK
- ✅ Next.js 官方推荐
- ✅ 支持滑动窗口、令牌桶等算法

**备选：Vercel KV**
- ✅ 如果部署在 Vercel，原生集成
- ❌ 绑定 Vercel 平台

**开发环境：内存缓存**
- ✅ 简单实现（Map + 定时清理）
- ❌ 仅限单机，不适合生产

#### **IP 获取方案**

```typescript
// 从请求头获取客户端 IP
function getClientIP(request: Request): string | null {
  return (
    request.headers.get('x-forwarded-for')?.split(',')[0] ||
    request.headers.get('x-real-ip') ||
    null
  )
}
```

---

### 实施建议

1. **先测量，再优化**
   - 在生产环境部署审计日志（优先级 2）
   - 观察 1-2 周，确认是否有扫描行为
   - 根据数据决定是否实施其他措施

2. **渐进式增强**
   - 不要一次性实施所有措施
   - 按优先级逐个实施，观察效果

3. **成本考量**
   - Upstash Redis 免费额度：10,000 请求/天
   - 如果流量较小，免费额度足够

---

## Phase 2: 用户面板与订阅系统开发任务

**更新时间**: 2025-12-13
**总工作量**: 5 天

### 阶段 1: 用户认证与面板 (2 天)

#### Day 1: 用户认证基础

- [x] **任务 1.1**: 扩展 Emby API 客户端
- **文件**: `lib/emby.ts`
- **工作量**: 2 小时
- **内容**:
  - 新增 `authenticateUser()` 方法（POST /Users/AuthenticateByName）
  - ~~新增 `updateUserPassword()` 方法~~（复用现有 `setUserPassword()`）
  - ~~新增 `updateUserEmail()` 方法~~（Emby 不支持，仅本地数据库）
- **验收**: ✅ 编译通过

- [x] **任务 1.2**: 实现用户认证 Server Actions
- **文件**: `app/actions/user-auth.ts`（新建）
- **工作量**: 2.5 小时
- **内容**:
  - 实现 `userLogin()` - 调用 Emby API 验证 + 生成 auth-token (role: 'user')
  - 实现 `getUserAuth()` - 从 auth-token cookie 获取当前用户
  - 实现 `userLogout()` - 删除 auth-token
  - 实现 `updateUserPassword()` - 修改密码并同步到 Emby
  - 实现 `updateUserEmail()` - 修改邮箱（仅本地数据库）
- **验收**: ✅ 所有 actions 已实现

- [x] **任务 1.3**: 更新 Middleware 路由保护
- **文件**: `middleware.ts`
- **工作量**: 0.5 小时
- **内容**:
  - 新增 `/user/*` 路由保护（使用统一的 auth-token）
  - 通过 role 字段区分管理员和用户（一套验证逻辑）
  - 已登录用户访问 `/user/login` 重定向到 `/user/dashboard`
- **验收**: ✅ 路由保护已实现

- [x] **任务 1.4**: 用户登录页面
- **文件**: `app/user/login/page.tsx`（新建，路径调整为非路由组）
- **工作量**: 2 小时
- **内容**:
  - 创建登录表单（用户名 + 密码）
  - 调用 `userLogin()` Server Action
  - 错误处理（显示错误信息）
  - 成功后跳转到 `/user/dashboard`
- **验收**: ✅ 登录页面已创建

---

#### Day 2: 用户仪表盘和账号管理

- [x] **任务 2.1**: 用户布局组件
- **文件**: `app/user/layout.tsx`（新建）
- **工作量**: 1 小时
- **内容**:
  - 创建用户导航栏（我的账号、我的订阅、登出）
  - 调用 `getUserAuth()` 获取当前用户
  - 未登录重定向到 `/user/login`
- **验收**: ✅ 导航栏已创建

- [x] **任务 2.2**: 用户仪表盘页面
- **文件**: `app/user/dashboard/page.tsx`（新建）
- **工作量**: 2 小时
- **内容**:
  - 显示用户信息（用户名、邮箱、Emby ID）
  - 显示到期时间、剩余天数
  - 显示账号状态（正常/已禁用）
  - 显示 Emby 服务器地址
- **验收**: ✅ 仪表盘页面已创建

- [x] **任务 2.3**: 修改密码功能
- **文件**: `app/user/dashboard/page.tsx`（扩展）
- **工作量**: 1.5 小时
- **内容**:
  - 创建修改密码表单（当前密码 + 新密码 + 确认新密码）
  - 调用 `updateUserPassword()` Server Action
  - 验证新密码强度（至少 6 个字符）
  - 成功提示、错误处理
- **验收**: ✅ 修改密码功能已集成

- [x] **任务 2.4**: 修改邮箱功能
- **文件**: `app/user/dashboard/page.tsx`（扩展）
- **工作量**: 1 小时
- **内容**:
  - 创建修改邮箱表单
  - 调用 `updateUserEmail()` Server Action
  - 验证邮箱格式
  - 成功提示、错误处理
- **验收**: ✅ 修改邮箱功能已集成

- [ ] **任务 2.5**: 测试用户认证流程
- **工作量**: 1 小时
- **测试内容**:
  - [ ] 用户可以用 Emby 账号密码登录
  - [ ] 用户可以查看账号信息
  - [ ] 用户可以修改密码（同步到 Emby）
  - [ ] 用户可以修改邮箱（仅本地数据库）
  - [ ] 已过期用户无法登录
  - [ ] 已禁用用户无法登录
  - [ ] 用户与管理员通过 role 字段隔离
- **验收**: 待测试

---

### 阶段 2: 订阅管理系统 (2 天)

#### Day 3: 数据库和 Server Actions

- [x] **任务 3.1**: 数据库 Schema 变更
- **文件**: `prisma/schema.prisma` + `prisma/migrations/add-subscription-table.sql`
- **工作量**: 30 分钟
- **内容**:
  - 新增 `Subscription` 表（添加 `mpError` 字段支持方案 A）
  - 修改 `User` 表（新增 `subscriptions` 关系）
  - ~~运行迁移~~（跳过，已提供 SQL 文件供手动执行）
  - 生成 Prisma Client：`npx prisma generate` ✅
- **验收**: ✅ Schema 已更新，SQL 迁移文件已创建

- [x] **任务 3.2**: 订阅 Server Actions（用户部分）
- **文件**: `app/actions/subscriptions.ts`（新建）
- **工作量**: 2.5 小时
- **内容**:
  - 实现 `createSubscription()` - 用户提交订阅
  - 实现 `getUserSubscriptions()` - 用户查看自己的订阅列表
  - 实现 `deleteSubscription()` - 用户删除 pending 订阅
- **验收**: ✅ 所有用户 Actions 已实现

- [x] **任务 3.3**: 订阅 Server Actions（管理员部分）
- **文件**: `app/actions/subscriptions.ts`（扩展）
- **工作量**: 2 小时
- **内容**:
  - 实现 `getAllSubscriptions()` - 管理员查看所有订阅
  - 实现 `approveSubscription()` - 管理员审核通过（暂不调用 MP API）
  - 实现 `rejectSubscription()` - 管理员拒绝订阅
- **验收**: ✅ 所有管理员 Actions 已实现

- [x] **任务 3.4**: 用户订阅列表页面
- **文件**: `app/user/subscriptions/page.tsx`（新建）
- **工作量**: 2 小时
- **内容**:
  - 显示用户的订阅列表（表格形式）
  - 显示：影视名称、类型、TMDB ID、状态、提交时间
  - pending 状态订阅可删除
  - approved/rejected 状态订阅不可操作
  - 显示同步失败状态（mpError 字段）
- **验收**: ✅ 订阅列表页面已创建

- [x] **任务 3.5**: 日志记录
- **工作量**: 1 小时
- **内容**:
  - 提交订阅时记录日志
  - 删除订阅时记录日志
  - 审核订阅时记录日志
- **验收**: ✅ 日志已集成在 Server Actions 中

---

#### Day 4: 订阅页面开发

**任务 4.1**: 提交新订阅页面
- **文件**: `app/(user)/subscriptions/new/page.tsx`（新建）
- **工作量**: 2 小时
- **内容**:
  - 创建表单：类型（电影/电视剧）、影视名称、TMDB ID、备注
  - 调用 `createSubscription()` Server Action
  - 表单验证（必填项检查）
  - 成功后跳转到订阅列表
- **验收**: 可以提交订阅

**任务 4.2**: 管理员审核页面
- **文件**: `app/admin/subscriptions/page.tsx`（新建）
- **工作量**: 3 小时
- **内容**:
  - 显示所有待审核订阅（可按状态筛选）
  - 显示：用户名、影视名称、类型、TMDB ID、提交时间
  - 审核通过/拒绝按钮
  - 搜索功能（按用户名或影视名称）
- **验收**: 管理员可以查看和审核订阅

**任务 4.3**: 管理员导航栏更新
- **文件**: `app/admin/layout.tsx`
- **工作量**: 30 分钟
- **内容**:
  - 新增"订阅管理"导航链接
- **验收**: 导航栏显示订阅管理

**任务 4.4**: 测试订阅管理流程
- **工作量**: 1.5 小时
- **测试内容**:
  - [ ] 用户可以提交订阅
  - [ ] 用户可以查看自己的订阅列表
  - [ ] 用户可以删除 pending 订阅
  - [ ] 用户无法删除 approved 订阅
  - [ ] 管理员可以查看所有订阅
  - [ ] 管理员可以审核通过订阅（暂不调用 MP API）
  - [ ] 管理员可以拒绝订阅
- **验收**: 所有测试通过

---

### 阶段 3: MoviePilot API 集成 (1 天)

#### Day 5: MoviePilot 集成

**任务 5.1**: MoviePilot API 客户端
- **文件**: `lib/moviepilot.ts`（新建）
- **工作量**: 1.5 小时
- **内容**:
  - 实现 `login()` - OAuth2 认证（每次调用都重新登录）
  - 实现 `createSubscription()` - 创建订阅
  - 错误处理（登录失败、API 调用失败）
- **验收**: 可以调用 MoviePilot API

**任务 5.2**: 更新审核 Server Action
- **文件**: `app/actions/subscriptions.ts`（修改 `approveSubscription()`）
- **工作量**: 1 小时
- **内容**:
  - 审核通过时调用 `moviepilotClient.createSubscription()`
  - 参数映射（movie → "电影"、tv → "电视剧"）
  - 错误处理（MP API 失败时返回错误信息）
  - 日志记录（包含 MP API 调用结果）
- **验收**: 审核通过后订阅提交到 MoviePilot

**任务 5.3**: 环境变量配置
- **文件**: `.env.example`
- **工作量**: 15 分钟
- **内容**:
  - 新增 `MOVIEPILOT_URL`
  - 新增 `MOVIEPILOT_USERNAME`
  - 新增 `MOVIEPILOT_PASSWORD`
- **验收**: 配置文件完整

**任务 5.4**: 完整流程测试
- **工作量**: 2 小时
- **测试内容**:
  - [ ] 用户提交订阅
  - [ ] 管理员审核通过
  - [ ] MoviePilot API 调用成功
  - [ ] 订阅状态变为 approved
  - [ ] MP API 失败时显示错误信息
  - [ ] 所有操作记录到日志
- **验收**: 端到端流程正常

**任务 5.5**: 文档更新
- **工作量**: 1.5 小时
- **内容**:
  - 更新 `docs/specs/design.md` 反映实际实现
  - 更新 `README.md` 新增 Phase 2 功能说明
  - 更新 `DEPLOYMENT.md` 新增 MoviePilot 配置说明
- **验收**: 文档完整准确

---

## 任务总结

### 文件清单

**新建文件（11 个）**:
- `app/actions/user-auth.ts` - 用户认证 Server Actions
- `app/actions/subscriptions.ts` - 订阅管理 Server Actions
- `app/(user)/login/page.tsx` - 用户登录页
- `app/(user)/dashboard/page.tsx` - 用户仪表盘
- `app/(user)/subscriptions/page.tsx` - 用户订阅列表
- `app/(user)/subscriptions/new/page.tsx` - 提交新订阅
- `app/(user)/layout.tsx` - 用户布局
- `app/admin/subscriptions/page.tsx` - 管理员审核页
- `lib/moviepilot.ts` - MoviePilot API 客户端
- `lib/user-auth-helpers.ts` - 用户认证辅助函数（如需）
- `lib/subscription-helpers.ts` - 订阅辅助函数（如需）

**修改文件（4 个）**:
- `lib/emby.ts` - 新增用户认证方法
- `middleware.ts` - 新增用户路由保护
- `prisma/schema.prisma` - 新增 Subscription 表
- `app/admin/layout.tsx` - 新增订阅管理导航

### 工作量统计

| 阶段   | 主要任务           | 工作量 | 关键文件数 |
|--------|-------------------|--------|-----------|
| 阶段 1 | 用户认证 + 仪表盘   | 2 天   | 5 个      |
| 阶段 2 | 订阅管理系统       | 2 天   | 4 个      |
| 阶段 3 | MoviePilot 集成    | 1 天   | 2 个      |
| **总计** |                   | **5 天** | **15 个** |

### 开发顺序建议

按照依赖关系，严格按以下顺序开发：

1. **Day 1**: 用户认证基础（Emby API + Server Actions + Middleware）
2. **Day 2**: 用户仪表盘和账号管理（登录页 + 仪表盘 + 修改密码/邮箱）
3. **Day 3**: 订阅数据模型和 Server Actions（数据库 + 后端逻辑）
4. **Day 4**: 订阅页面开发（用户订阅页 + 管理员审核页）
5. **Day 5**: MoviePilot 集成（MP API + 完整流程测试）

---

**Phase 2 任务拆分完成，等待实施。**
