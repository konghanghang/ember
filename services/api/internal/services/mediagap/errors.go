package mediagap

import "errors"

var (
	ErrMediaGapNotConfigured = errors.New("缺集管理所需的 Emby 或 TMDB 配置未完成")
	ErrMediaGapInvalidStatus = errors.New("缺集状态无效")
	ErrMediaGapNotFound      = errors.New("缺集工单不存在")
	ErrMediaGapInvalidID     = errors.New("缺集工单 ID 不能为空")
	ErrMediaGapCandidate     = errors.New("候选资源不能为空")
	ErrMediaGapSearchState   = errors.New("当前缺集状态不支持搜索")
	ErrMediaGapDispatchState = errors.New("当前缺集状态不支持下发")
)
