# Changelog

All notable changes to the Ember project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [1.0.0] - 2025-12-13

### 🎉 MVP 版本发布

第一个生产可用版本，包含核心用户管理功能。

### ✨ 新功能

#### 管理员功能
- ✅ 管理员登录（JWT 认证）
- ✅ 邀请码管理（生成、查看、删除）
- ✅ 用户管理（查看、禁用/启用、删除、延长有效期）
- ✅ 系统设置（修改密码）

#### 用户功能
- ✅ 用户注册（邀请码验证）
- ✅ 自动创建 Emby 账号
- ✅ 账号有效期管理

#### 系统功能
- ✅ 定时任务（自动禁用过期用户）
- ✅ 数据库迁移
- ✅ Docker 容器化部署
- ✅ CI/CD 自动化流程

### 🔧 技术栈

- **前端**: Next.js 15 App Router + React 19
- **后端**: Next.js Server Actions
- **数据库**: PostgreSQL 16 + GORM
- **UI**: Tailwind CSS + shadcn/ui
- **部署**: Docker + Docker Compose

### 🐛 重大修复

详见 [docs/archive/BUGFIX-SUMMARY.md](docs/archive/BUGFIX-SUMMARY.md)

1. **[严重] 数据一致性问题** - 修复用户注册时 Emby 创建失败导致的脏数据
2. **[严重] 并发问题** - 修复邀请码并发使用导致超限的竞态条件
3. **[严重] 权限控制** - 修复登录验证逻辑错误
4. **[中等] Docker 环境变量** - 修复 NEXT_PUBLIC_EMBY_URL 在 Docker 中不生效
5. **[中等] 定时任务可靠性** - 增强定时任务的错误处理和日志
6. **[轻微] 管理员密码默认值** - 改进初始密码的安全性提示

### 📚 文档

- ✅ [设计文档](docs/specs/design.md)
- ✅ [部署指南](docs/runbooks/DEPLOYMENT.md)
- ✅ [开发指南](docs/reference/development-guide.md)
- ✅ [测试指南](docs/runbooks/TESTING.md)

### ✅ 测试覆盖

- **核心功能测试**: 20/20 通过
- **详细报告**: 历史测试报告已归档到 `docs/archive/test-reports/`

### 🚀 部署方式

支持三种部署模式：
- **模式零**: 使用预构建镜像（最快）
- **模式一**: 生产部署（本地构建 + 远程数据库）
- **模式二**: 本地开发（本地构建 + 本地数据库）

详见 [docs/runbooks/DEPLOYMENT.md](docs/runbooks/DEPLOYMENT.md)

### 🔐 安全特性

- ✅ JWT 令牌认证
- ✅ bcrypt 密码加密
- ✅ Emby API Key 环境变量保护
- ✅ Docker 非 root 用户运行
- ✅ SQL 注入防护（GORM 参数化查询）

### 📦 Docker 镜像

- **镜像大小**: 351MB（纯运行时）
- **多阶段构建**: ✅
- **健康检查**: ✅
- **自动重启**: ✅

### ⚠️ 已知限制

- 邮件通知功能未实现（计划 Phase 2）
- MoviePilot 集成未实现（计划 Phase 2）
- 批量用户操作未实现（计划 Phase 2）

### 📝 升级说明

首次发布，无需升级。

---

## [Unreleased]

### 计划功能（Phase 2）

- [ ] 邮件通知（注册成功、即将到期、已到期）
- [ ] MoviePilot 集成
- [ ] 批量用户管理
- [ ] 用户自助面板
- [ ] Redis 缓存

---

## 版本格式说明

- **[Major.Minor.Patch]** - 语义化版本号
  - **Major**: 重大不兼容变更
  - **Minor**: 新功能，向后兼容
  - **Patch**: Bug 修复，向后兼容

- **标签**:
  - `✨ 新功能` - 新增功能
  - `🐛 修复` - Bug 修复
  - `🔧 优化` - 性能优化或代码重构
  - `📚 文档` - 文档更新
  - `🔐 安全` - 安全修复
  - `⚠️ 破坏性变更` - 不兼容的变更

---

**项目主页**: https://github.com/yourusername/ember
**文档**: [docs/README.md](docs/README.md)
