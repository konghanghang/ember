package services

import "errors"

var (
	ErrRedemptionCodeNotFound   = errors.New("兑换码不存在")
	ErrRedemptionCodeInvalid    = errors.New("兑换码已失效")
	ErrRedemptionCodeUsedOver   = errors.New("最大使用次数不能小于已使用次数")
	ErrSettingNotFound          = errors.New("配置项不存在")
	ErrRedeemFailed             = errors.New("兑换失败，请稍后重试")
	ErrEmbyUnbanFailed          = errors.New("Emby 解封失败，请稍后重试")
	ErrSubscriptionDuplicated   = errors.New("该影片已提交订阅，请勿重复提交")
	ErrSubscriptionNotFound     = errors.New("订阅不存在")
	ErrEmailNotConfigured       = errors.New("邮件服务未配置")
	ErrEmailAlreadyRegistered   = errors.New("邮箱已被注册")
	ErrEmailNotRegistered       = errors.New("该邮箱未注册")
	ErrEmailCodeRateLimit       = errors.New("该邮箱今日发送次数已达上限")
	ErrEmailCodeIPRateLimit     = errors.New("请求过于频繁，请稍后再试")
	ErrEmailCodeInvalid         = errors.New("邮箱验证码无效或已过期")
	ErrEmailSendFailed          = errors.New("验证码发送失败，请稍后重试")
	ErrPlanNotFound             = errors.New("方案不存在")
	ErrPaymentFailed            = errors.New("支付处理失败")
	ErrTelegramAlreadyBound     = errors.New("该 Telegram 账号已绑定其他用户")
	ErrTelegramBindCodeInvalid  = errors.New("绑定验证码无效或已过期")
	ErrTelegramNotBound         = errors.New("尚未绑定 Telegram 账号")
	ErrUserAlreadyBoundTelegram = errors.New("该账号已绑定 Telegram")
)
