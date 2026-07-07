package subscription

import "errors"

var (
	ErrSubscriptionDuplicated      = errors.New("该作品已提交订阅，请勿重复提交")
	ErrSubscriptionNotFound        = errors.New("订阅不存在")
	ErrSubscriptionForbidden       = errors.New("无权操作此订阅")
	ErrSubscriptionDeleteForbidden = errors.New("无权删除此订阅")
	ErrSubscriptionInvalidSeason   = errors.New("season 不能小于 0")
	ErrSubscriptionHandled         = errors.New("订阅已被处理")
	ErrSubscriptionDeleteState     = errors.New("只能删除待审核的订阅")
	ErrSubscriptionRejectReason    = errors.New("拒绝原因不能为空")
	ErrSubscriptionNotApproved     = errors.New("只能手动处理已通过且待入库的订阅")
	ErrSubscriptionNotInLibrary    = errors.New("Emby 库中未找到对应已入库资源")
	ErrSubscriptionNotRejected     = errors.New("只能重新提交已拒绝的订阅")
	ErrSubscriptionResubmitNote    = errors.New("本次提交说明不能为空")
	ErrSubscriptionStateConflict   = errors.New("订阅状态已变更，请刷新后重试")
	ErrSubscriptionRedispatchSafe  = errors.New("当前订阅没有可重试的 MoviePilot 失败状态")
	ErrSubscriptionManualSeason    = errors.New("整剧订阅需要先指定季号")
	ErrSubscriptionManualCandidate = errors.New("请选择要下发的候选资源")
	ErrSubscriptionInvalidTMDBID   = errors.New("订阅的 TMDB ID 无效或为空")
	ErrSubscriptionEmbyUnlinked    = errors.New("当前账号未绑定 Emby，不能提交订阅")
	ErrSubscriptionEmbyDisabled    = errors.New("当前账号的 Emby 已被禁用，不能提交订阅")
)
