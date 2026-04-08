package payment

import "errors"

var (
	ErrPlanNotFound             = errors.New("方案不存在")
	ErrPlanCurrencyInvalid      = errors.New("方案币种无效，仅支持 USD/HKD/CNY")
	ErrPlanGroupInvalid         = errors.New("套餐分组标识无效")
	ErrPlanGroupNotFound        = errors.New("套餐分组不存在")
	ErrDefaultPlanGroupNotFound = errors.New("默认套餐分组不存在")
	ErrPlanGroupDeleteBlocked   = errors.New("套餐分组仍被用户或套餐引用，不能删除")
	ErrDefaultPlanGroupDelete   = errors.New("默认套餐分组不能删除")
	ErrPaymentFailed            = errors.New("支付处理失败")
	ErrStripeNotConfigured      = errors.New("Stripe 支付未配置")
	ErrEmbyUnbanFailed          = errors.New("Emby 解封失败，请稍后重试")
)
