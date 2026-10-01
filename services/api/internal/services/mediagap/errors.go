package mediagap

import "errors"

var (
	ErrMediaGapDeleteState   = errors.New("仅允许删除已入库或已忽略的工单，请刷新后重试")
	ErrMediaGapDeleteIDs     = errors.New("请选择 1 到 100 条有效工单")
	ErrMediaGapStateConflict = errors.New("缺集工单状态已变化，请刷新后重试")
	ErrMediaGapNotConfigured = errors.New("缺集管理所需的 Emby 或 TMDB 配置未完成")
	ErrMediaGapInvalidStatus = errors.New("缺集状态无效")
	ErrMediaGapNotFound      = errors.New("缺集工单不存在")
	ErrMediaGapInvalidID     = errors.New("缺集工单 ID 不能为空")
	ErrMediaGapCandidate     = errors.New("候选资源不能为空")
	ErrMediaGapSearchState   = errors.New("当前缺集状态不支持搜索")
	ErrMediaGapDispatchState = errors.New("当前缺集状态不支持下发")
)
