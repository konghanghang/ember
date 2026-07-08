# 集成测试手册

这份手册只回答一件事：如何在 Ember 仓库中实际运行集成测试、查看执行结果，并为后续 `Web + API` 联合集成测试预留统一入口。

## 1. 适用范围

适用场景：

- 你要跑 API 进程内集成测试
- 你要连接真实 PostgreSQL 跑后端业务流测试
- 你准备落地 `Web + API` 联合集成测试
- 你希望保留每个测试用例的执行结果，而不是只看终端一行通过/失败

如果你只是改了一个小函数，先看 [测试指南](./testing.md)。

## 2. 当前现状

当前仓库已具备：

- API 路由级进程内集成测试骨架
  - `services/api/internal/app/integration_test_helpers_test.go`
  - `services/api/internal/app/integration_plan_groups_test.go`
- Web 页面级集成测试
  - `services/web/src/**/*.spec.ts`
- Bot 测试目录
  - `services/bot/tests/`
- 统一本地测试结果脚本入口
  - `scripts/test/api.sh`
  - `scripts/test/web.sh`
  - `scripts/test/bot.sh`
  - `scripts/test/all.sh`

当前还未完全成型，但建议按本手册逐步收口：

- `services/api/internal/integration/`
- `services/e2e/`
- `artifacts/test-results/`

## 3. 环境准备

### 3.1 API 集成测试数据库

API 集成测试默认需要显式提供测试 PostgreSQL 连接串：

```bash
export EMBER_INTEGRATION_DATABASE_URL='postgres://user:pass@127.0.0.1:5432/ember_test?sslmode=disable'
```

约束：

- 只能指向测试数据库
- 不要复用开发主库
- 更不能指向生产库
- 不要指向共享测试库；本地协作时应优先给集成测试单独建一个专用 database

API 集成测试骨架会为每个用例创建独立 schema，并在结束后自动清理。

推荐做法：

- 为集成测试单独创建一个 database，例如 `ember_integration`
- 通过 `EMBER_INTEGRATION_DATABASE_URL` 显式注入
- 测试只在这个专用 database 内创建和删除 schema，不碰共享测试库

### 3.2 必要环境变量

当前 API 集成测试骨架会自行设置：

- `EMBER_MIGRATIONS_DIR`
- `JWT_SECRET`

后续如某条业务流需要更多配置，再在测试中局部补齐，不要把所有环境变量硬塞到全局。

## 4. 当前可执行入口

### 4.1 API 进程内集成测试

运行全部 `internal/app` 测试：

```bash
cd services/api
go test ./internal/app -count=1
```

只跑集成测试命名集合：

```bash
cd services/api
EMBER_INTEGRATION_DATABASE_URL='postgres://user:pass@127.0.0.1:5432/ember_test?sslmode=disable' \
go test ./internal/app -run Integration -count=1
```

说明：

- 未设置 `EMBER_INTEGRATION_DATABASE_URL` 时，集成测试会自动 `Skip`
- 这能避免普通开发时被误伤

### 4.2 Web 页面级集成测试

```bash
cd services/web
npm run test
```

按页面定向执行：

```bash
cd services/web
npm run test -- src/views/admin/PlanGroupsView.spec.ts src/views/admin/UsersView.spec.ts src/views/console/AccountCenterView.spec.ts
```

### 4.3 Bot 测试

```bash
cd services/bot
python3.11 -m venv .venv
source .venv/bin/activate
python -m py_compile main.py
python -m pytest tests
```

约定：

- Bot 默认使用 `services/bot/.venv/bin/python`
- 统一脚本 `scripts/test/bot.sh` 和 `make test-bot-report` 也按这个路径执行
- 如果 `.venv` 不存在，脚本会直接失败并提示创建方式，而不是退回系统 Python

## 5. 结果产物建议

不要把测试结果记在 Markdown 里。应该让命令直接产出机器可读报告。

推荐目录：

```text
artifacts/test-results/
  api/
  web/
  bot/
  e2e/
```

推荐产物：

- `junit.xml`
- `json-summary.json`

当前仓库约定：

- 统一产物根目录：`artifacts/test-results/`
- API：`artifacts/test-results/api/`
- Web：`artifacts/test-results/web/`
- Bot：`artifacts/test-results/bot/`
- 未来 E2E：`artifacts/test-results/e2e/`

## 6. 推荐执行命令

### 6.1 API 结果报告

当前推荐直接使用统一脚本：

```bash
make test-api-report
```

脚本行为：

- `go vet ./...`
- `go test ./...`
- `go build ./...`
- 如已设置 `EMBER_INTEGRATION_DATABASE_URL`，再补 API 集成测试

如果本地已安装 `gotestsum`，脚本会额外输出 JUnit；否则至少保留 `go test -json` 结果。

手动执行的等价命令仍然可以保留，建议后续统一到脚本：

```bash
cd services/api
gotestsum \
  --format standard-verbose \
  --junitfile ../../artifacts/test-results/api/junit.xml \
  --jsonfile ../../artifacts/test-results/api/results.json \
  ./...
```

如果只跑 API 集成测试：

```bash
cd services/api
EMBER_INTEGRATION_DATABASE_URL='postgres://user:pass@127.0.0.1:5432/ember_test?sslmode=disable' \
gotestsum \
  --format standard-verbose \
  --junitfile ../../artifacts/test-results/api/integration-junit.xml \
  --jsonfile ../../artifacts/test-results/api/integration-results.json \
  ./internal/app -run Integration -count=1
```

### 6.2 Web 结果报告

当前推荐直接使用统一脚本：

```bash
make test-web-report
```

脚本会输出：

- `artifacts/test-results/web/vitest-report.json`
- `artifacts/test-results/web/vitest.log`
- `artifacts/test-results/web/build.log`

等价命令示例：

```bash
cd services/web
vitest run \
  --reporter=default \
  --reporter=junit \
  --outputFile.junit=../../artifacts/test-results/web/junit.xml
```

如果需要 JSON 汇总，建议在后续统一脚本里补齐。

### 6.3 Bot 结果报告

当前推荐直接使用统一脚本：

```bash
make test-bot-report
```

脚本会输出：

- `artifacts/test-results/bot/junit.xml`
- `artifacts/test-results/bot/pytest.log`
- `artifacts/test-results/bot/py-compile.log`

等价命令示例：

```bash
cd services/bot
python -m pytest tests \
  --junitxml=../../artifacts/test-results/bot/junit.xml
```

## 7. 如何查看每个测试用例执行情况

你要看的不是“命令退没退出 0”，而是每个 case 的具体状态。

至少要能看到：

- 用例名
- 所属测试文件或 suite
- `passed / failed / skipped`
- 耗时
- 失败堆栈

推荐查看方式：

- 本地直接看终端详细输出
- CI 中看 JUnit 解析结果
- 本地或 CI 汇总 JSON 结果

后续如果你要做“全局回归总览”，建议再加一个汇总脚本，把 API、Web、Bot、E2E 的结果合并成一份摘要。

## 8. `Web + API` 联合集成测试建议

这层当前还没有正式目录，但建议直接按下面约定落：

```text
services/e2e/
  playwright.config.ts
  tests/
    auth/
    media-library-policy/
    payment/
    subscription/
```

推荐原则：

- Web 真页面
- API 真服务
- PostgreSQL 真库
- 外部系统默认 fake

第一批最值得落地的流：

1. 管理员保存 `PlanGroup` 媒体库模板为“仅保存模板”
2. 用户账号中心显示 `待同步`
3. 用户点击“同步到 Emby”
4. 页面状态收口为 `已同步`

## 9. 全局回归建议

全局回归不要只靠一条命令，建议维护两个集合。

### 9.1 最小全局回归

适合提交前或高频改动后执行：

```bash
make test
```

如果已经建设了 API 集成测试和 E2E，再补：

```bash
cd services/api
EMBER_INTEGRATION_DATABASE_URL='...' go test ./internal/app -run Integration -count=1
```

如果你同时需要保留结果产物，直接执行：

```bash
make test-report
```

### 9.2 全量回归

适合发布前或夜间任务：

- API 全量测试
- Web 全量测试
- Bot 全量测试
- API 集成测试全集
- `Web + API` 联合集成测试全集
- 按需手工 smoke

## 10. 扩展规则

后续新增集成测试时，统一遵守：

- 先放进对应层级目录
- 再补执行入口
- 最后补结果产物输出

不要出现“测试已经写了，但只有作者自己知道怎么跑”的情况。

## 11. 相关文档

- [测试策略](../reference/testing-strategy.md)
- [测试指南](./testing.md)
- [手工测试清单](./manual-testing-checklist.md)
- [测试排障](./testing-troubleshooting.md)
