package subscription

import "errors"

var (
	ErrSubscriptionDuplicated    = errors.New("该作品已提交订阅，请勿重复提交")
	ErrSubscriptionNotFound      = errors.New("订阅不存在")
	ErrSubscriptionInvalidSeason = errors.New("season 不能小于 0")
	ErrSubscriptionHandled       = errors.New("订阅已被处理")
	ErrSubscriptionRejectReason  = errors.New("拒绝原因不能为空")
)
