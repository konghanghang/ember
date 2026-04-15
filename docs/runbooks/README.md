# 操作手册

这里放“怎么做”的文档，目标是让部署、测试、构建和联调可重复执行。

## 文档列表

- [部署指南](./deployment.md) - Docker 部署入口与最小启动路径
- [部署环境与配置](./deployment-environment.md) - 必填变量、迁移策略、管理员初始化
- [数据库 Migration Baseline](./database-migration-baseline.md) - baseline 生成、验证与旧迁移归档流程
- [部署排障](./deployment-troubleshooting.md) - 部署失败时的检查动作和恢复动作
- [测试指南](./testing.md) - 测试入口与最短验证路径
- [手工测试清单](./manual-testing-checklist.md) - 按变更范围选跑的手工回归清单
- [测试排障](./testing-troubleshooting.md) - 编译、联调、集成测试常见阻塞
- [Stripe 支付测试指南](./stripe-payment-testing.md) - Stripe CLI 与 cloudflared 两种支付联调方式
- [Cloudflared 本地联调](./cloudflared-local-testing.md) - Telegram Webhook 本地联调
- [Docker 构建指南](./docker-build-guide.md) - 镜像构建与本地 build 使用方式
- [发布流程](./release-process.md) - `pre_release`、Tag 与 Draft Release 流程

## 维护规则

- 只写可执行步骤，不写抽象原则。
- 入口页负责导航，专项细节拆出去，不要再回到“大而全单文件”。
- 如果文档里的命令、路径、环境变量已经变了，必须立即更新；操作手册不能容忍“差不多”。
