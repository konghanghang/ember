# Telegram 集成

Ember 可以通过 Telegram Bot 提供通知和自助能力。

## 主要能力

- 新用户注册通知
- 求片订阅审批通知
- 支付成功通知
- 播放排行通知
- 用户绑定账号后执行自助命令

## 用户侧命令

- `/bind`
- `/info`
- `/redeem`
- `/resetpw`
- `/search`

## 部署前需要准备

- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_WEBHOOK_SECRET`
- `WEBHOOK_URL`
- `INTERNAL_API_SECRET`

通常还会配置：

- `TELEGRAM_ADMIN_CHAT_ID`
- `TELEGRAM_GROUP_CHAT_ID`

## 行为说明

- Bot 使用 webhook 模式接收消息
- 部分运行期设置会通过 API 动态读取
- 群欢迎消息依赖 `notify_group_link`
- 群菜单默认不公开展示，需要管理员按规则刷新

## 继续阅读

- [配置说明](../configuration.md)
- [常见问题](../faq.md)
