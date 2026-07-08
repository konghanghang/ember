# 测试策略

本文档定义 Ember 项目的长期测试分层、覆盖边界和回归原则。它描述的是“我们应该怎样测试”，而不是某一次测试怎么执行。

## 1. 目标

这套测试策略要解决三个问题：

- 改动后如何快速判断回归风险，而不是只看编译是否通过。
- 如何把 API、Web、Bot、数据库和外部集成的测试分层收口，不让测试体系继续碎片化。
- 如何保留所有测试用例，并在后续全局回归时得到可追踪、可汇总的执行结果。

## 2. 总体原则

- 单层测试不能证明“系统整体没问题”；全局回归必须由多层测试组合完成。
- 默认不启动项目服务，不调用真实 Emby、TMDB、MoviePilot、Stripe、Telegram；外部依赖优先使用 fake server、mock client 或隔离测试环境。
- 测试代码本身也是长期资产，必须按职责分层组织，而不是把所有断言都堆到单测里。
- 执行结果由机器产物承载，不用 Markdown 人工记录每次通过/失败状态。
- 高风险改动优先补“业务流集成测试”，不是只补工具函数单测。

## 3. 测试分层

### 3.1 单元测试

目标：

- 验证纯函数、DTO 转换、状态判断、边界条件和错误分支。

特点：

- 无真实数据库。
- 无真实 HTTP 路由。
- 无真实外部系统。

适用：

- 数据归一化
- 规则判断
- 小型服务方法
- 格式化与解析逻辑

### 3.2 契约测试

目标：

- 锁定与外部系统的请求/响应契约，不依赖真实外部服务。

特点：

- 使用 `httptest.NewServer` 或等价 fake HTTP server。
- 断言 method、path、query、header、body 和兼容字段分支。

适用：

- Emby API
- TMDB API
- MoviePilot API
- Bot Internal API

### 3.3 API 进程内集成测试

目标：

- 验证真实 HTTP 路由、middleware、handler、service、数据库写入和本地状态收口是否能串起来。

特点：

- 使用真实 `gin` router。
- 通过 `httptest.NewRecorder()` 发送 HTTP 请求。
- 使用真实 PostgreSQL 测试库或独立 schema。
- 外部系统仍使用 fake server 或 fake client。

适用：

- 登录、注册、兑换、续期
- 用户状态流转
- 媒体库模板与 Policy 同步
- 管理后台核心保存链路
- Bot Internal API

### 3.4 真 PostgreSQL 集成测试

目标：

- 验证 migration、GORM 行为、唯一约束、事务回滚、状态聚合 SQL 等真实依赖数据库语义的逻辑。

特点：

- 必须连接真实 PostgreSQL。
- 每个测试用例或测试套件使用独立 schema 或独立 database。
- 启动前自动跑 migration。

适用：

- partial unique index
- batch/task 状态聚合
- 查询列表派生字段
- 事务一致性
- migration / VerifySchema

### 3.5 Web 页面级集成测试

目标：

- 验证真实页面在 mock API 下的交互、状态切换、按钮显隐、错误提示和关键用户路径。

特点：

- 使用 `Vitest + Vue Test Utils`。
- 页面级优先，组件级次之。
- 不依赖真实后端。

适用：

- 管理后台关键页面
- 账号中心
- 支付中心
- 订阅流

### 3.6 Web + API 联合集成测试

目标：

- 验证高频业务流在真实 Web、真实 API、真实 PostgreSQL 组合下是否工作正常。

特点：

- Web 指向测试 API。
- API 指向测试 PostgreSQL。
- 外部系统默认仍用 fake server。
- 适合用浏览器自动化工具统一驱动。

适用：

- 登录后后台操作流
- 分组模板保存与用户侧状态收口
- 支付中心高频流程
- 订阅审批与回显

### 3.7 隔离环境冒烟测试

目标：

- 验证部署后关键链路能在接近真实环境中跑通。

特点：

- 使用隔离环境。
- 允许连接测试 Emby、测试 Telegram、Stripe sandbox、测试 MoviePilot。
- 不纳入日常开发默认测试入口。

适用：

- 发布前 smoke
- 外部集成变更
- 部署、配置、Webhook、反向代理改动

## 4. 目录与归类约定

当前推荐约定：

- `services/api/internal/.../*_test.go`
  - 单元测试
  - 契约测试
  - 轻量路由测试
- `services/api/internal/app/`
  - API 进程内集成测试骨架
- `services/api/internal/integration/`
  - 真 PostgreSQL 或跨组件集成测试
- `services/web/src/**/*.spec.ts`
  - 页面级集成测试
- `services/e2e/`
  - `Web + API` 联合集成测试
- `services/bot/tests/`
  - Bot 单元、契约与进程内集成测试

如果新增测试目录，必须先回答两个问题：

1. 它属于哪一层测试。
2. 它的执行入口会不会与现有层级冲突。

## 5. 回归策略

### 5.1 日常开发

- 优先跑改动附近的测试文件。
- 然后跑所属子系统的最短验证路径。

### 5.2 提交前

- 跑所属子系统全量测试。
- 涉及状态流转、数据库 schema、外部集成时，补该领域的集成测试或手工回归。

### 5.3 全局回归

全局回归不是一条命令，而是一组测试集合：

- API 全量测试
- Web 全量测试
- Bot 全量测试
- 指定的 API 集成测试集合
- 指定的 `Web + API` 联合集成测试集合

必须维护两套集合：

- 最小回归集合
  - 高频、核心、提交前可接受时长
- 全量回归集合
  - 发布前、夜间任务或专项验证使用

## 6. 高风险改动的最低测试要求

### 用户状态流转

至少覆盖：

- API 进程内集成测试
- 真 PostgreSQL 集成测试
- 必要的手工回归

### 数据库 schema / migration

至少覆盖：

- migration 骨架测试
- VerifySchema 验证
- 真 PostgreSQL 集成测试

### 外部集成链路

至少覆盖：

- 契约测试
- API 进程内集成测试
- 隔离环境冒烟

### Web 管理后台关键页面

至少覆盖：

- 页面级集成测试
- 受影响 API 的进程内集成测试

## 7. 外部依赖策略

默认策略：

- Emby：fake server
- TMDB：fake server
- MoviePilot：fake server
- Telegram Bot API：fake client 或 fake HTTP server
- Stripe：fake webhook / sandbox 只在隔离环境使用

禁止项：

- 不在 `go test`、`npm test`、`pytest` 中访问真实生产服务。
- 不用代理、真实 token、真实 webhook 回调替代缺失 mock。

## 8. 测试结果与可观测性

测试用例“保留”靠代码和报告，不靠人工笔记。

必须区分两类资产：

- 测试代码
  - 仓库内长期保留
- 测试执行结果
  - 以机器产物形式保存

推荐统一产物格式：

- `JUnit XML`
- `JSON Summary`

推荐产物目录：

```text
artifacts/test-results/
  api/
  web/
  bot/
  e2e/
```

要求：

- 每次回归应能定位到每个 test case 的通过、失败、跳过和耗时。
- 回归失败时，应能快速关联到所属测试层和测试文件。
- 测试报告目录默认不提交到 git，由 CI 或本地脚本生成。

## 9. 文档与脚本的职责分离

- `docs/reference/testing-strategy.md`
  - 说明测试分层、边界和长期策略
- `docs/runbooks/testing.md`
  - 说明最短验证路径
- `docs/runbooks/integration-testing.md`
  - 说明如何执行集成测试与查看结果
- `scripts/test/`
  - 固定执行入口，避免团队成员各自手写命令

## 10. 当前优先建设顺序

建议按下面顺序推进：

1. API 进程内集成测试骨架
2. 真 PostgreSQL 集成测试骨架
3. 媒体库 / Policy / 用户状态流转的第一批 API 集成流
4. `Web + API` 联合集成测试骨架
5. 统一测试报告产物和汇总脚本

## 11. 维护规则

- 新增高风险功能时，先决定它应该落在哪一层测试，再开始补用例。
- 新增测试入口或结果目录时，同步更新 `docs/runbooks/` 中的执行手册。
- 如果某层测试已经长期失效或无人运行，应优先修复或删除，不保留僵尸测试层。
