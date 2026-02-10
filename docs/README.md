# Ember 项目文档

> Ember MVP - 核心用户管理系统文档中心

**⚠️ 上线前必读**: [Bug 修复总结](specs/archive/BUGFIX-SUMMARY.md) - 6 个严重问题已修复 ✅

---

## 🚀 快速上手（3 步）

新用户从这里开始：

1. **[设计文档](specs/design.md)** - 了解 MVP 核心功能、系统架构和实现细节
2. **[开发规范](DEVELOPMENT.md)** - 环境搭建、代码规范、数据库设计决策
3. **[部署指南](DEPLOYMENT.md)** - 一键部署到生产环境

---

## 👨‍💻 开发者指南

开发和维护 Ember 必读：

### 核心文档
- **[开发规范](DEVELOPMENT.md)** - 环境搭建、代码规范、数据库设计决策
- **[Emby API 参考](emby-api-guide.md)** - Emby 集成文档和 API 使用示例
- **[部署指南](DEPLOYMENT.md)** - 包含 CI/CD 流程和 GitHub Actions 配置

### 开发任务
- **[任务拆分](specs/archive/tasks.md)** - 开发任务列表（已完成，已归档）

---

## 🧪 测试验收

确保系统质量：

- **[测试指南](TESTING.md)** - 完整测试步骤、环境准备、测试清单和故障排查（✅ 20/20 通过）
- **[测试报告](specs/archive/test-reports/)** - 历史测试记录（已归档）

---

## 📚 历史归档

<details>
<summary>点击展开：已归档的历史文档</summary>

这些文档已归档至 `specs/archive/` 目录，仅供参考：

- **[开发任务列表](specs/archive/tasks.md)** - 完整的开发任务规划（已完成）
- **[Bug 修复总结](specs/archive/BUGFIX-SUMMARY.md)** - 重大 Bug 修复记录
- **[测试报告](specs/archive/test-reports/)** - 历史测试记录

**注意**：当前实现以 `specs/design.md` 和主要文档为准。

</details>

---

## 📋 文档结构

```
docs/
├── README.md                          # 📍 本文档（导航入口）
│
├── 🚀 快速上手
│   ├── specs/design.md                # MVP 需求 + 设计文档（数据库 + API）
│   ├── DEVELOPMENT.md                # 开发规范（环境搭建、代码规范、数据库设计）
│   └── DEPLOYMENT.md                # 部署指南（含 CI/CD 流程）
│
├── 👨‍💻 开发指南
│   ├── DEVELOPMENT.md                # 开发规范
│   ├── emby-api-guide.md             # Emby 集成文档
│   └── DEPLOYMENT.md                # 部署指南（含 CI/CD）
│
├── 🧪 测试验收
│   └── TESTING.md                    # 测试完整指南（含测试清单）
│
└── 📚 specs/archive/（历史归档）
    ├── tasks.md                      # 开发任务列表（已完成）
    ├── BUGFIX-SUMMARY.md             # 重大 Bug 修复总结
    └── test-reports/                # 历史测试记录
```

---

## 🎯 文档索引

### 按角色查找

| 角色 | 推荐文档 |
|------|---------|
| **新用户/产品经理** | specs/design.md |
| **部署运维** | DEPLOYMENT.md |
| **前端开发** | DEVELOPMENT.md → specs/design.md (API 部分) |
| **后端开发** | DEVELOPMENT.md → specs/design.md → emby-api-guide.md |
| **测试人员** | TESTING.md |

### 按任务查找

| 任务 | 文档 |
|------|------|
| 搭建开发环境 | DEVELOPMENT.md |
| 部署到生产 | DEPLOYMENT.md |
| 了解 API 设计 | specs/design.md (Server Actions 部分) |
| 集成 Emby | emby-api-guide.md |
| 配置 CI/CD | DEPLOYMENT.md |
| 执行测试 | TESTING.md |

---

## 🔄 文档维护

### 更新频率

| 文档类型 | 更新时机 |
|---------|---------|
| **specs/** | 需求或设计变更时立即更新 |
| **DEVELOPMENT.md** | 开发流程变更时更新 |
| **DEPLOYMENT.md** | 部署流程或配置变更时更新 |
| **TESTING.md** | 新增测试用例或流程变更时更新 |
| **specs/archive/BUGFIX-SUMMARY.md** | 发现和修复重大 Bug 时更新 |

### 文档规范

**命名规范**：
- 核心文档使用大写：`README.md`, `DEVELOPMENT.md`, `DEPLOYMENT.md`, `TESTING.md`
- 其他文档使用小写和连字符：`emby-api-guide.md`
- 目录使用小写：`specs/`, `specs/archive/`

**结构规范**：
```markdown
# 文档标题

> 简短描述（一句话）

## 概述
...

## 详细内容
...

## 相关文档
...

---

**文档更新日期**: YYYY-MM-DD
```

---

## 💡 贡献指南

欢迎改进文档！提交 PR 时请确保：

- ✅ 使用 Markdown 格式，保持风格一致
- ✅ 添加必要的代码示例和截图
- ✅ 更新文档底部的"文档更新日期"
- ✅ 在相关文档中添加交叉引用链接

---

## 🆘 获取帮助

- **GitHub Issues**: https://github.com/yourusername/ember/issues
- **讨论区**: https://github.com/yourusername/ember/discussions

---

**文档最后更新**: 2026-02-10

**MVP 状态**: ✅ 核心功能已完成，✅ Phase 2 功能已完成，测试通过 20/20

---

**文档优化完成**：从 15 个文档（8936行）→ 8 个文档（7018行）
