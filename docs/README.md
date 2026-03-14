# Ember 项目文档

> Ember — Emby 用户管理系统文档中心

---

## 文档索引

| 文档 | 用途 | 受众 |
|------|------|------|
| **[系统架构](SYSTEM-ARCHITECTURE.md)** | 数据模型、服务逻辑、API 端点、前端结构 | 所有开发者 |
| **[配置参考](CONFIGURATION-REFERENCE.md)** | 配置来源、密钥用途、数据库/环境变量边界 | 开发/运维 |
| **[API 响应规范](API-RESPONSE-STANDARD.md)** | JSON 格式、字段命名、GORM 映射约定 | 后端开发 |
| **[部署指南](DEPLOYMENT.md)** | Docker 部署、环境变量、CI/CD | 运维 |
| **[测试指南](TESTING.md)** | 测试步骤、环境准备、故障排查 | 测试 |
| **[Cloudflared 本地联调](CLOUDFLARED-LOCAL-TESTING.md)** | Telegram webhook 本地联调与 Surge 排障 | 开发/测试 |
| **[Emby API 参考](emby-api-guide.md)** | Emby 集成接口、调试技巧 | 后端开发 |

---

## 快速开始

1. 阅读 **[系统架构](SYSTEM-ARCHITECTURE.md)** 了解整体设计
2. 参考 **[部署指南](DEPLOYMENT.md)** 搭建环境
3. 按照 **[测试指南](TESTING.md)** 验证功能

---

## 归档文档

历史文档存放在 `specs/archive/`，仅供参考：

- `specs/archive/plan/` — 已完成的功能实施计划
- `specs/archive/API-REFERENCE.md` — 旧版 API 详细文档（Next.js 时期）
- `specs/archive/BUGFIX-SUMMARY.md` — 重大 Bug 修复记录
- `specs/archive/tasks.md` — 开发任务列表（已完成）
- `specs/archive/test-reports/` — 历史测试报告
- `specs/design.md` — MVP 初始需求文档

---

## 目录结构

```
docs/
├── README.md                    # 本文档（导航入口）
├── SYSTEM-ARCHITECTURE.md       # 系统架构（核心参考）
├── CONFIGURATION-REFERENCE.md   # 配置参考
├── API-RESPONSE-STANDARD.md     # API 响应规范
├── DEPLOYMENT.md                # 部署指南
├── TESTING.md                   # 测试指南
├── CLOUDFLARED-LOCAL-TESTING.md # Cloudflared 本地联调
├── emby-api-guide.md            # Emby API 参考
└── specs/
    ├── design.md                # MVP 需求文档
    └── archive/                 # 历史归档
```
