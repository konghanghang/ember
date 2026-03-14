package services

import "errors"

var (
	ErrPlanNotFound = errors.New("方案不存在")
	ErrPaymentFailed = errors.New("支付处理失败")
)
