# Ember MVP - 核心用户管理系统 - Requirements Document

Ember 的最小可行产品（MVP）版本，专注于核心功能：邀请码注册、用户管理、到期控制。砍掉了所有非必要功能（MoviePilot、邮件通知、批量操作、操作审计等），目标是 2 周内上线一个可用的系统。

## Core Features

### 1. 管理员认证
- 管理员使用用户名 + 密码登录
- 使用 JWT Token 认证（7 天有效期）
- Token 存储在 localStorage
- 保护所有管理后台路由

### 2. 邀请码管理
- 生成随机 8 位邀请码（如 `a7K9bX2Q`）
- 设置邀请码属性：
  - 最大使用次数（默认 1 次）
  - 可选的过期时间
  - 默认账号到期天数（如 30 天）
- 查看邀请码列表（显示使用情况）
- 删除未使用的邀请码

### 3. 用户注册
- 用户通过邀请码注册
- 填写：用户名、密码、邮箱
- 系统自动：
  - 验证邀请码有效性
  - 调用 Emby API 创建用户
  - 保存用户信息到数据库
  - 设置账号到期时间
- 注册成功后返回 Emby 服务器地址

### 4. 用户管理
- 查看用户列表：
  - 用户名、邮箱
  - 创建时间
  - 到期时间（显示剩余天数）
  - 账号状态（正常/已禁用）
- 搜索用户（按用户名）
- 延长账号到期时间
- 禁用/启用账号（同步到 Emby）
- 删除用户（同时删除 Emby 用户）

### 5. 账号到期处理
- 定时任务（每天凌晨 2:00）：
  - 扫描已到期账号
  - 调用 Emby API 禁用账号
  - 更新数据库状态
- 手动触发到期检查（管理后台）

### 6. 基础配置
- Emby 服务器配置（URL + API Key）
- 连接状态检测
- 默认用户权限（固定配置，不可修改）

## User Stories

### 管理员
- **作为管理员**，我要登录系统，以便管理用户
- **作为管理员**，我要生成邀请码，以便让新用户注册
- **作为管理员**，我要查看用户列表，以便了解用户状态
- **作为管理员**，我要延长用户到期时间，以便给用户续期
- **作为管理员**，我要禁用/删除用户，以便管理不活跃用户

### 用户
- **作为用户**，我要使用邀请码注册，以便获得 Emby 账号
- **作为用户**，我要知道账号何时到期，以便及时续期

## Acceptance Criteria

### 管理员登录
- [ ] 使用错误密码登录失败，返回错误提示
- [ ] 使用正确密码登录成功，返回 JWT Token
- [ ] Token 过期后自动跳转登录页
- [ ] 未登录访问管理页面自动跳转登录

### 邀请码生成
- [ ] 生成的邀请码为 8 位随机字符串（大小写字母+数字）
- [ ] 邀请码不重复
- [ ] 设置最大使用次数后，使用次数达到上限时邀请码失效
- [ ] 设置过期时间后，过期邀请码无法使用

### 用户注册
- [ ] 使用无效邀请码注册失败
- [ ] 用户名已存在时注册失败
- [ ] 注册成功后在 Emby 能看到新用户
- [ ] 注册成功后数据库有对应记录
- [ ] 邀请码使用次数正确更新

### 用户管理
- [ ] 用户列表正确显示所有用户
- [ ] 搜索功能能找到匹配的用户
- [ ] 延长到期时间后，数据库和显示都更新
- [ ] 禁用账号后，Emby 用户无法登录
- [ ] 删除用户后，Emby 用户也被删除

### 账号到期
- [ ] 定时任务每天执行一次
- [ ] 到期账号被正确禁用
- [ ] 未到期账号不受影响
- [ ] 手动触发检查功能正常

## Non-functional Requirements

### 性能要求
- 用户列表加载时间 < 1 秒（1000 用户以内）
- 注册流程完成时间 < 3 秒
- Emby API 调用失败时有重试机制（最多 3 次）

### 安全要求
- 管理员密码使用 bcrypt 加密存储（cost=10）
- JWT Token 使用强随机密钥
- 所有 API 调用验证 Token
- Emby API Key 不在前端暴露

### 兼容性要求
- 支持 PostgreSQL 14+
- 支持 Emby Server 4.7+
- 前端兼容现代浏览器（Chrome 90+, Firefox 88+, Safari 14+）

### 可靠性要求
- Emby API 调用失败时，事务回滚（不创建半成品用户）
- 定时任务异常不影响系统运行
- 数据库连接失败时有友好提示

### 可维护性要求
- 代码使用 TypeScript 严格模式
- 关键操作有日志记录（登录、注册、删除用户）
- 配置通过环境变量管理，不硬编码

## Out of Scope (明确不做的功能)

以下功能**不在 MVP 范围内**，等 MVP 上线后根据反馈再决定：

- ❌ 邮件通知（欢迎邮件、到期提醒）
- ❌ MoviePilot 集成
- ❌ 批量操作（批量删除、批量续期）
- ❌ 权限配置模板（使用固定默认权限）
- ❌ 用户自助面板（见 Phase 2 规划）
- ❌ 操作审计日志（只记录基础日志）
- ❌ 设备管理（查看/踢出设备）
- ❌ 统计仪表盘（用户数、活跃度）
- ❌ Webhook 支持
- ❌ 多管理员
- ❌ 数据库备份功能

## Phase 2 规划（基于真实用户反馈）

**背景**：MVP 已上线，根据真实用户反馈规划 Phase 2 功能。

---

### 优先级 P0：用户自助面板

**需求来源**：5 位用户强烈要求

**功能描述**：
用户可以使用 Emby 账号密码登录 Ember，查看自己的账号信息并管理个人设置。

**核心功能**：

1. **用户认证**
   - 使用 Emby 账号密码登录（不是 Ember 独立密码）
   - 验证账号是否有效（未过期、未禁用）
   - 生成用户专用 JWT Token（user-token cookie，与管理员 auth-token 分离）
   - Token 有效期 7 天

2. **账号信息查看**
   - 显示：用户名、邮箱、Emby ID
   - 显示：到期时间、剩余天数
   - 显示：账号状态（正常/已禁用）
   - 显示：Emby 服务器地址

3. **密码和邮箱管理**
   - 修改密码（同步到 Emby，验证当前密码后允许修改）
   - 修改邮箱（同步到 Emby，如果 Emby API 支持）
   - 修改本地数据库的 User 记录

**新增路由**：
- `/user/login` - 用户登录页
- `/user/dashboard` - 用户仪表盘
- `/user/subscriptions` - 用户订阅列表（Phase 2.2）
- `/user/subscriptions/new` - 提交新订阅（Phase 2.2）

**验收标准**：
- [ ] 用户可以用 Emby 账号密码登录 Ember
- [ ] 用户可以查看自己的账号信息（用户名、邮箱、到期时间）
- [ ] 用户可以查看剩余天数
- [ ] 用户可以修改密码，修改后立即在 Emby 生效
- [ ] 用户可以修改邮箱，同步到本地数据库和 Emby
- [ ] 已过期用户无法登录
- [ ] 已禁用用户无法登录
- [ ] 用户只能看到自己的数据（路由保护）
- [ ] 用户 Token 与管理员 Token 完全隔离（不同 cookie 名称）

**开发时间估算**：2 天

---

### 优先级 P0：MoviePilot 订阅集成

**需求来源**：用户主要使用场景，强烈需求

**功能描述**：
用户可以提交影视订阅请求，管理员审核后自动提交到 MoviePilot。

**核心功能**：

1. **用户提交订阅**
   - 方式一：输入 TMDB ID（快速，推荐）
   - 方式二：搜索影视名称（未来优化）
   - 必填信息：
     - 类型（movie / tv）
     - 影视名称
     - TMDB ID
   - 可选信息：
     - 年份
     - 用户备注
   - 提交后状态为 "pending"（待审核）

2. **用户管理订阅**
   - 查看自己的订阅列表（按提交时间倒序）
   - 删除 pending 状态的订阅（已审核的不可删除）
   - 查看订阅状态：pending / approved / rejected

3. **管理员审核订阅**
   - 查看所有待审核订阅（可按状态筛选）
   - 显示用户信息、影视名称、TMDB ID
   - 审核通过：调用 MoviePilot API 创建订阅
   - 审核拒绝：订阅状态变为 rejected

4. **MoviePilot API 集成**
   - 使用 OAuth2 认证（用户名密码 → access_token）
   - 调用 `POST /api/v1/subscribe/` 创建订阅
   - 参数映射：
     - movie → "电影"
     - tv → "电视剧"
   - 错误处理：API 失败时显示明确错误信息

**订阅状态机**：
```
pending → (approve) → approved (immutable)
        ↘ (reject) → rejected (terminal)
```

**新增路由**：
- `/user/subscriptions` - 用户订阅列表
- `/user/subscriptions/new` - 提交新订阅
- `/admin/subscriptions` - 管理员审核页面

**验收标准**：
- [ ] 用户可以输入 TMDB ID 提交订阅
- [ ] 用户可以查看自己的订阅列表
- [ ] 用户可以删除 pending 状态的订阅
- [ ] 用户无法删除 approved/rejected 状态的订阅
- [ ] 管理员可以查看所有待审核订阅
- [ ] 管理员可以审核通过订阅（调用 MP API）
- [ ] 管理员可以拒绝订阅
- [ ] MP API 调用成功后订阅状态变为 approved
- [ ] MP API 调用失败时显示明确错误信息
- [ ] 所有关键操作记录到日志表

**开发时间估算**：3 天（2 天订阅系统 + 1 天 MP 集成）

---

### 优先级 P1：未来增强功能（等 Phase 2.1 完成后决定）

- **邮件通知**：到期提醒（用户可能更喜欢 Telegram/微信）
- **批量操作**：管理员批量续期/删除（取决于用户量）
- **统计仪表盘**：用户活跃度分析（取决于数据需求）
- **Jellyfin 集成**：有用户使用 Jellyfin 而不是 Emby（取决于反馈）

## Technical Constraints

### 必须使用的技术
- **框架**: Next.js 15 (App Router)
- **语言**: TypeScript 5.x (严格模式)
- **数据库**: PostgreSQL 16+
- **ORM**: Prisma
- **认证**: 自实现 JWT（不使用 NextAuth.js）
- **UI**: shadcn/ui + Tailwind CSS
- **定时任务**: node-cron

### 部署方式
- Docker 单容器部署
- 使用 docker-compose 管理服务

### 数据库设计约束
- 只需要 4 张表：
  1. `admins` - 管理员表
  2. `invites` - 邀请码表
  3. `users` - 用户表
  4. `logs` - 简单日志表（可选）

## Success Metrics

MVP 成功的标准：

1. **功能完整性**: 6 个核心功能全部实现并测试通过
2. **开发周期**: 2 周内完成（10 个工作日）
3. **代码质量**: TypeScript 编译无错误，无明显性能问题
4. **可部署性**: 能通过 `docker-compose up` 一键部署
5. **可用性**: 能完成完整的用户注册-使用-到期流程

## Development Plan

### Week 1 (Day 1-5)
- **Day 1-2**: 项目初始化 + 数据库设计 + 管理员登录
- **Day 3-4**: 邀请码生成 + 用户注册（含 Emby 集成）
- **Day 5**: 用户列表 + 延长到期时间

### Week 2 (Day 6-10)
- **Day 6-7**: 禁用/删除用户 + 定时任务
- **Day 8**: Docker 部署 + 配置管理
- **Day 9-10**: 测试 + Bug 修复 + 文档

## Risks and Mitigation

### 风险 1: Emby API 不稳定
- **影响**: 用户注册/删除失败
- **缓解**: 实现重试机制，失败时回滚数据库操作

### 风险 2: 时间估算不准
- **影响**: 2 周内无法完成
- **缓解**: 已砍掉所有非核心功能，保留最小功能集

### 风险 3: PostgreSQL 数据丢失
- **影响**: 用户数据丢失
- **缓解**: 提醒用户配置 PostgreSQL 数据卷持久化（docker-compose）

## Appendix

### Emby API 端点（需要使用）
- `POST /Users/New` - 创建用户
- `DELETE /Users/{userId}` - 删除用户
- `POST /Users/{userId}/Policy` - 更新用户权限
- `GET /Users` - 获取用户列表

### 默认用户权限配置
```json
{
  "IsAdministrator": false,
  "IsDisabled": false,
  "EnableAllFolders": true,
  "EnabledFolders": [],
  "EnableRemoteAccess": true,
  "EnableLiveTvAccess": false,
  "EnableContentDeletion": false,
  "EnableContentDownloading": false,
  "EnableSyncTranscoding": false,
  "EnableMediaPlayback": true,
  "EnableAudioPlaybackTranscoding": true,
  "EnableVideoPlaybackTranscoding": true,
  "EnablePlaybackRemuxing": true
}
```
