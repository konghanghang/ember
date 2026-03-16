# Stripe 集成

Ember 使用 Stripe 提供一次性支付能力。

## 当前能力

- 管理员维护付费方案
- 用户创建支付会话
- Stripe 回调后自动履约
- 支付成功后延长账号有效期

## 关键配置

- `STRIPE_SECRET_KEY`
- `STRIPE_WEBHOOK_SECRET`
- `STRIPE_SUCCESS_URL`
- `STRIPE_CANCEL_URL`

## 说明

- 当前不是周期订阅，而是一次性购买订阅天数
- 如果未配置 Stripe，在线支付能力应视为关闭

## 继续阅读

- [支付与续费](../features/payments.md)
- [配置说明](../configuration.md)
