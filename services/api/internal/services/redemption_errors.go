package services

import "errors"

var (
	ErrRedemptionCodeNotFound = errors.New("兑换码不存在")
	ErrRedemptionCodeInvalid  = errors.New("兑换码已失效")
	ErrRedemptionDuplicate    = errors.New("你已经使用过此兑换码")
	ErrRedemptionCodeUsedOver = errors.New("最大使用次数不能小于已使用次数")
	ErrRedeemFailed           = errors.New("兑换失败，请稍后重试")
	ErrEmbyUnbanFailed        = errors.New("Emby 解封失败，请稍后重试")
)
