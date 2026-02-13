package services

import "errors"

var (
	ErrRedemptionCodeNotFound = errors.New("兑换码不存在")
	ErrRedemptionCodeInvalid  = errors.New("兑换码已失效")
	ErrRedemptionCodeUsedOver = errors.New("最大使用次数不能小于已使用次数")
	ErrSettingNotFound        = errors.New("配置项不存在")
	ErrRedeemFailed           = errors.New("兑换失败，请稍后重试")
	ErrEmbyUnbanFailed        = errors.New("Emby 解封失败，请稍后重试")
	ErrSubscriptionDuplicated = errors.New("该影片已提交订阅，请勿重复提交")
	ErrSubscriptionNotFound   = errors.New("订阅不存在")
)
