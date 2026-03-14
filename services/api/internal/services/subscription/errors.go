package subscription

import "errors"

var (
	ErrSubscriptionDuplicated = errors.New("该影片已提交订阅，请勿重复提交")
	ErrSubscriptionNotFound   = errors.New("订阅不存在")
)
