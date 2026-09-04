package payment

import "errors"

var (
	ErrPlanNotFound                                      = errors.New("方案不存在")
	ErrPlanCurrencyInvalid                               = errors.New("方案币种无效，仅支持 USD/HKD/CNY")
	ErrPlanNameRequired                                  = errors.New("方案名称不能为空")
	ErrPlanGroupInvalid                                  = errors.New("套餐分组标识无效")
	ErrPlanGroupNotFound                                 = errors.New("套餐分组不存在")
	ErrPlanGroupNameRequired                             = errors.New("套餐分组名称不能为空")
	ErrPlanGroupKeyExists                                = errors.New("套餐分组标识已存在")
	ErrPlanGroupSubscriptionAutoApproveDailyLimitInvalid = errors.New("每日自动通过订阅数不能小于 0")
	ErrPlanGroupP115PlaybackModeInvalid                  = errors.New("115 播放账号来源无效")
	ErrPlanGroupP115TransferHourlyLimitInvalid           = errors.New("115 每小时转存配额必须在 1 到 100 之间")
	ErrPlanGroupP115TransferDailyLimitInvalid            = errors.New("115 每日转存配额必须在 1 到 1000 之间")
	ErrDefaultPlanGroupNotFound                          = errors.New("默认套餐分组不存在")
	ErrDefaultPlanGroupRequired                          = errors.New("默认套餐分组不能为空，请先设置其他默认分组")
	ErrPlanGroupDeleteBlocked                            = errors.New("套餐分组仍被用户、套餐或注册码引用，不能删除")
	ErrDefaultPlanGroupDelete                            = errors.New("默认套餐分组不能删除")
	ErrPaymentFailed                                     = errors.New("支付处理失败")
	ErrStripeNotConfigured                               = errors.New("Stripe 支付未配置")
	ErrEmbyUnbanFailed                                   = errors.New("Emby 解封失败，请稍后重试")
	ErrStripeWebhookInvalid                              = errors.New("webhook 签名验证失败")
	ErrStripeWebhookParseFailed                          = errors.New("解析 webhook 数据失败")
)
