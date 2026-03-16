# Ember 文档中心

这里是 Ember 的唯一文档导航入口。规则很简单：先判断文档属于“稳定事实”还是“阶段性方案”，再决定放哪。

## 阅读顺序

1. [系统架构](./SYSTEM-ARCHITECTURE.md)：先建立系统全局认知。
2. [开发指南](./reference/development-guide.md)：再看开发时真正要遵守的入口规则。
3. 按用途进入对应分区：`reference`、`runbooks`、`proposals`、`archive`。

## 文档分层

| 分区 | 作用 | 放什么 |
|------|------|--------|
| [SYSTEM-ARCHITECTURE.md](./SYSTEM-ARCHITECTURE.md) | 核心真相来源 | 当前系统结构、数据模型、服务边界、API 端点 |
| [reference/](./reference/README.md) | 稳定参考文档 | 规范、配置、外部接口、长期有效约束 |
| [runbooks/](./runbooks/README.md) | 操作手册 | 部署、测试、构建、联调、排障 |
| [proposals/](./proposals/README.md) | 方案与设计 | 重构提案、功能规划、需求设计 |
| [archive/](./archive/README.md) | 历史归档 | 已完成、已废弃或仅供追溯的旧文档 |

## 当前入口

### 核心

- [系统架构](./SYSTEM-ARCHITECTURE.md)
- [开发指南](./reference/development-guide.md)

### 稳定参考

- [配置参考](./reference/CONFIGURATION-REFERENCE.md)
- [API 开发与目录规范](./reference/API-DEVELOPMENT-CONVENTIONS.md)
- [API 响应规范](./reference/API-RESPONSE-STANDARD.md)
- [Emby API 参考](./reference/emby-api-guide.md)
- [Web 设计规范](./reference/WEB_DESIGN_GUIDE.md)

### 操作手册

- [部署指南](./runbooks/DEPLOYMENT.md)
- [测试指南](./runbooks/TESTING.md)
- [Cloudflared 本地联调](./runbooks/CLOUDFLARED-LOCAL-TESTING.md)
- [Docker 构建指南](./runbooks/DOCKER-BUILD-GUIDE.md)

### 方案与设计

- [提案总览](./proposals/README.md)
- [API 目录重构提案](./proposals/API-DIRECTORY-REFACTOR.md)
- [`docs/plan/`](./plan/)：进行中的功能规划，保持原路径以兼容协作流程
- [`docs/specs/`](./specs/)：`specs-workflow` 产物，当前仍保留在原路径

### 归档

- [归档总览](./archive/README.md)
- [MVP 初始设计](./specs/design.md)

## 维护规则

- 新增稳定规则：放进 `reference/`。
- 新增操作流程：放进 `runbooks/`。
- 新增需求或设计讨论：先放 `plan/` 或 `specs/`，落地后提炼，再归档。
- 已经失效但需要保留追溯：放进 `archive/`。
