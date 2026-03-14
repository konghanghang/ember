package payment

import "errors"

var (
	ErrPlanNotFound        = errors.New("方案不存在")
	ErrPaymentFailed       = errors.New("支付处理失败")
	ErrStripeNotConfigured = errors.New("Stripe 支付未配置")
	ErrEmbyUnbanFailed     = errors.New("Emby 解封失败，请稍后重试")
)
