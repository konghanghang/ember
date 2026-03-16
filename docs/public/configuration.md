# 配置说明

这份文档只列公开部署最常见的配置项，不展开内部配置治理细节。

## 核心配置

### API 与数据库

- `DATABASE_URL`
- `JWT_SECRET`
- `CONFIG_ENCRYPTION_KEY`
- `ADMIN_USERNAME`
- `ADMIN_PASSWORD`

### Emby

- `EMBY_URL`
- `NEXT_PUBLIC_EMBY_URL`
- `EMBY_API_KEY`

### Telegram Bot

- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_WEBHOOK_SECRET`
- `TELEGRAM_ADMIN_CHAT_ID`
- `TELEGRAM_GROUP_CHAT_ID`
- `WEBHOOK_URL`
- `INTERNAL_API_SECRET`

### 可选集成

- `TMDB_API_KEY`
- `MOVIEPILOT_URL`
- `MOVIEPILOT_USERNAME`
- `MOVIEPILOT_PASSWORD`
- `STRIPE_SECRET_KEY`
- `STRIPE_WEBHOOK_SECRET`

## 选择原则

- 先补齐核心配置，再启用可选集成
- 对外地址和内部服务地址不要混用
- 所有密钥都应替换默认示例值

## 继续阅读

- [部署说明](./deployment.md)
- [Telegram 集成](./integrations/telegram.md)
- 更多内部配置边界请参考仓库中的开发文档
