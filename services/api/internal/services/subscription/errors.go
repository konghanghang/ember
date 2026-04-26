package subscription

import "errors"

var (
	ErrSubscriptionDuplicated     = errors.New("该作品已提交订阅，请勿重复提交")
	ErrSubscriptionNotFound       = errors.New("订阅不存在")
	ErrSubscriptionForbidden      = errors.New("无权操作此订阅")
	ErrSubscriptionInvalidSeason  = errors.New("season 不能小于 0")
	ErrSubscriptionHandled        = errors.New("订阅已被处理")
	ErrSubscriptionRejectReason   = errors.New("拒绝原因不能为空")
	ErrSubscriptionNotApproved    = errors.New("只能手动处理已通过且待入库的订阅")
	ErrSubscriptionNotInLibrary   = errors.New("Emby 库中未找到对应已入库资源")
	ErrSubscriptionNotRejected    = errors.New("只能重新提交已拒绝的订阅")
	ErrSubscriptionResubmitNote   = errors.New("本次提交说明不能为空")
	ErrSubscriptionStateConflict  = errors.New("订阅状态已变更，请刷新后重试")
	ErrSubscriptionRedispatchSafe = errors.New("当前订阅没有可重试的 MoviePilot 失败状态")
)
