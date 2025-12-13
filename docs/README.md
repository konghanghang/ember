# Ember 项目文档

> Ember MVP - 核心用户管理系统文档中心

**⚠️ 上线前必读**: [Bug 修复总结](BUGFIX-SUMMARY.md) - 6 个严重问题已修复 ✅

---

## 🚀 快速上手（3 步）

新用户从这里开始：

1. **[需求文档](specs/requirements.md)** - 了解 MVP 核心功能
2. **[设计文档](specs/design.md)** - 理解系统架构和实现细节
3. **[部署指南](DEPLOYMENT.md)** - 一键部署到生产环境

---

## 👨‍💻 开发者指南

开发和维护 Ember 必读：

### 核心文档
- **[开发规范](development-guide.md)** - 环境搭建、代码规范、数据库设计决策
- **[Emby API 参考](emby-api-guide.md)** - Emby 集成文档和 API 使用示例
- **[CI/CD 指南](cicd-guide.md)** - 自动化部署流程和 GitHub Actions 配置

### 开发任务
- **[任务拆分](specs/tasks.md)** - 开发任务列表（Day 1-9 已完成）

---

## 🧪 测试验收

确保系统质量：

- **[测试检查清单](testing-checklist.md)** - 20 项核心功能 ✅ 20/20 通过
- **[测试指南](testing-guide.md)** - 完整测试步骤、环境准备和故障排查
- **[测试报告](test-reports/)** - 历史测试记录

---

## 📚 设计历史（参考）

<details>
<summary>点击展开：设计演化过程和技术决策</summary>

### 设计阶段文档（已归档）

这些文档记录了 Ember 从概念到 MVP 的完整设计过程：

- **[项目总结](archive/00-summary.md)** - Phase 1/2/3 规划概览
- **[完整需求](archive/01-requirements.md)** - 包含未来功能的完整需求文档
- **[完整架构](archive/02-architecture.md)** - 全面的系统架构设计
- **[技术决策](archive/tech-stack-decision.md)** - 为什么选择 Next.js 全栈架构

**注意**：这些文档已归档至 `archive/` 目录，仅供参考。当前实现以 `specs/` 目录下的文档为准。

</details>

---

## 📋 文档结构

```
docs/
├── README.md                          # 📍 本文档（导航入口）
│
├── 🚀 快速上手
│   ├── specs/requirements.md          # MVP 需求文档
│   ├── specs/design.md                # 设计文档（数据库 + API）
│   └── DEPLOYMENT.md                  # 部署指南
│
├── 👨‍💻 开发指南
│   ├── development-guide.md           # 开发规范
│   ├── emby-api-guide.md              # Emby 集成
│   ├── cicd-guide.md                  # CI/CD 流程
│   └── specs/tasks.md                 # 任务拆分
│
├── 🧪 测试验收
│   ├── testing-checklist.md           # 测试清单
│   ├── testing-guide.md               # 测试指南
│   └── test-reports/                  # 测试报告
│
├── 🐛 问题修复
│   └── BUGFIX-SUMMARY.md              # 重大 Bug 修复总结
│
└── 📚 archive/（设计历史）
    ├── 00-summary.md                  # 项目总览
    ├── 01-requirements.md             # 完整需求
    ├── 02-architecture.md             # 完整架构
    └── tech-stack-decision.md         # 技术选型
```

---

## 🎯 文档索引

### 按角色查找

| 角色 | 推荐文档 |
|------|---------|
| **新用户/产品经理** | requirements.md → design.md |
| **部署运维** | DEPLOYMENT.md → cicd-guide.md |
| **前端开发** | development-guide.md → design.md (API 部分) |
| **后端开发** | development-guide.md → design.md → emby-api-guide.md |
| **测试人员** | testing-guide.md → testing-checklist.md |

### 按任务查找

| 任务 | 文档 |
|------|------|
| 搭建开发环境 | development-guide.md |
| 部署到生产 | DEPLOYMENT.md |
| 了解 API 设计 | design.md (Server Actions 部分) |
| 集成 Emby | emby-api-guide.md |
| 配置 CI/CD | cicd-guide.md |
| 执行测试 | testing-guide.md |

---

## 🔄 文档维护

### 更新频率

| 文档类型 | 更新时机 |
|---------|---------|
| **specs/** | 需求或设计变更时立即更新 |
| **development-guide.md** | 开发流程变更时更新 |
| **DEPLOYMENT.md** | 部署流程或配置变更时更新 |
| **testing-*.md** | 新增测试用例或流程变更时更新 |
| **BUGFIX-SUMMARY.md** | 发现和修复重大 Bug 时更新 |

### 文档规范

**命名规范**：
- 使用小写和连字符：`development-guide.md`
- 目录使用小写：`specs/`, `test-reports/`
- 核心文档使用大写：`README.md`, `DEPLOYMENT.md`

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

**文档最后更新**: 2025-12-13

**MVP 状态**: ✅ 核心功能已完成，测试通过 20/20
