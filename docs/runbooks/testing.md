# 测试指南

这份文档只保留测试入口和最短执行路径，不再把历史测试报告和整套旧栈说明塞进来。

## 何时看这份文档

- 你改了 API、Web、Bot，需要知道最低验证动作
- 你改了集成配置、鉴权、支付、Telegram，需要知道还要补哪些手工检查
- 你要决定是做“编译级验证”还是“完整手工回归”

## 最短验证路径

### API 改动

```bash
cd services/api
go vet ./...
go test ./...
go build ./...
```

Playback Gateway 相关改动额外运行：

```bash
cd services/api
go test -count=1 ./internal/integrations/emby ./internal/playbackgateway
go test -race -count=1 ./internal/integrations/emby ./internal/playbackgateway ./internal/services/embytoken
go test -count=1 ./internal/entrypoint ./internal/app
go build ./cmd/ember
```

115 personal/system 路由与 Redis 配额的默认测试使用进程内 fake Redis、`httptest` fake Emby/115，不需要真实服务。专项入口：

```bash
cd services/api
go test -count=1 ./internal/playbackgateway ./internal/services/directplay ./internal/services/p115account ./internal/services/p115quota
go test -race -count=1 ./internal/playbackgateway ./internal/services/directplay ./internal/services/p115account ./internal/services/p115quota
```

专项测试必须覆盖 personal/system 账号路由、Redis reservation/active/paused/Stopped、HEAD 不创建、小时/自然日转存额度、Redis 断连 fallback，以及成功 `302` 不访问 Emby、DirectPlay 失败无损回到 Emby。不得把 fake Redis 替换为真实 Redis、Emby、115 或 CloudDrive2 调用。

上述命令只做 fake 上游、生命周期、统一子命令分发和构建验证，不启动 API/Gateway、不请求真实 Emby。默认构建验证不会在工作区生成二进制；需要单独产物时显式使用 `go build -o bin/ember ./cmd/ember`，且禁止提交 `bin/`。

如果要跑本地 API 集成测试：

- 必须通过 `EMBER_INTEGRATION_DATABASE_URL` 指向专用测试数据库

截至 2026-09-05，已在专用 PostgreSQL 集成环境执行并通过：

```bash
go test ./internal/app -run 'Integration|PostgreSQL|P115' -count=1 -v
```

该测试会由 harness 创建和清理独立的 `itest_*` schema；不会启动 Ember 服务，也不会访问真实 Emby、115 或其他外部系统。若环境变量缺失，相关测试会跳过，不能将跳过写成通过。

`docker compose config --quiet` 属于部署者上线前的运行前检查，不是本地代码测试的必要条件；应在目标部署环境使用实际 `.env` 和 Compose override 单独执行。
- 不要直接连接共享开发库，尤其不要复用其他人正在使用的测试库
- 集成测试骨架会在目标数据库里创建并清理独立 schema，所以目标库本身应只用于集成测试
- 115 migration 用例会重复执行当前增量，并覆盖套餐默认值、账号 partial unique、owner `ON DELETE RESTRICT`、revoked tombstone 与 transfer provenance；未设置该变量时会明确跳过，不能据此声称 PostgreSQL migration 已实际执行

示例：

```bash
cd services/api
EMBER_INTEGRATION_DATABASE_URL='postgres://user:pass@127.0.0.1:5432/ember_integration?sslmode=disable' \
go test ./internal/app -run Integration -count=1
```

如果需要保留每个测试用例的执行结果：

```bash
make test-api-report
```

### Web 改动

```bash
cd services/web
npm ci
npm run test
npm run build
```

如果需要保留测试结果产物：

```bash
make test-web-report
```

### Bot 改动

```bash
cd services/bot
python3.11 -m venv .venv
source .venv/bin/activate
pip install -r requirements-dev.txt
python -m py_compile main.py
python -m pytest tests
```

约定：

- Bot 测试与本地运行默认使用 `services/bot/.venv`
- 当前仓库通过 `services/bot/.python-version` 固定 pyenv Python `3.11.15`；本机验证已执行 `.venv/bin/python -m py_compile main.py` 和 `.venv/bin/python -m pytest tests`，49 项测试全部通过
- 如果 `.venv` 不存在，`make setup`、`make test-bot`、`make test-bot-report` 会直接提示先创建虚拟环境

如果需要保留测试结果产物：

```bash
make test-bot-report
```

## 什么时候必须补手工测试

下列改动只跑编译不够：

- 登录、注册、兑换、账号状态流转
- 管理后台关键页面和设置保存
- Emby、TMDB、MoviePilot 集成
- Telegram 绑定、通知、Webhook
- 支付流程
- Docker Compose、环境变量、反向代理、部署脚本

手工测试项见 [手工测试清单](./manual-testing-checklist.md)。

## 历史测试报告怎么处理

现行指南不再内嵌历史报告。需要追溯旧测试结论时，去看：

- [历史测试报告：2025-12-07](../archive/report/test/2025-12-07-mvp-core-testing.md)

## 常见误区

- 文档改动不会触发 `.github/workflows/test.yml`，因为 CI 对 `docs/**` 和 `*.md` 做了 `paths-ignore`
- `go build ./...` 通过，不代表 Emby、Telegram、支付链路真的可用
- 手工测试不是“把所有页面点一遍”，而是按变更范围跑对应清单

## 继续阅读

- [测试策略](../reference/testing-strategy.md)
- [集成测试手册](./integration-testing.md)
- [手工测试清单](./manual-testing-checklist.md)
- [115 Cookie Provider 一次性只读合同验证](./p115-read-only-contract-check.md)
- [115 playback 保留式秒传合同验证](./p115-retained-transfer-contract-check.md)
- [Stripe 支付测试指南](./stripe-payment-testing.md)
- [测试排障](./testing-troubleshooting.md)
- [部署指南](./deployment.md)
- [Cloudflared 本地联调](./cloudflared-local-testing.md)
