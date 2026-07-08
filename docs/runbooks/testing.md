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
- [Stripe 支付测试指南](./stripe-payment-testing.md)
- [测试排障](./testing-troubleshooting.md)
- [部署指南](./deployment.md)
- [Cloudflared 本地联调](./cloudflared-local-testing.md)
