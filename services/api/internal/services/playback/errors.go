package playback

import "errors"

var (
	ErrPlaybackHistoryInvalidDate    = errors.New("日期格式错误，应为 YYYY-MM-DD")
	ErrPlaybackHistoryInvalidKeyword = errors.New("keyword 仅支持中英文、数字、空格和 -_.'，且长度不能超过 100")
	ErrPlaybackHistoryInvalidUserID  = errors.New("userId 格式无效")
	ErrPlaybackHistoryUserNotFound   = errors.New("用户不存在")
	ErrPlaybackHistoryQueryFailed    = errors.New("Playback Reporting 查询失败")
	ErrPlaybackHistorySchemaUnsupported = errors.New("Playback Reporting 字段结构不受支持，请升级插件或缩小查询范围")
	ErrPlaybackProfileInvalidDate    = errors.New("startDate/endDate 必须是 YYYY-MM-DD 或 YYYY-MM-DD HH:mm:ss 格式，且需同时传入")
	ErrPlaybackProfileInvalidRange   = errors.New("range 仅支持 today、7d、30d、90d、all")
	ErrPlaybackProfileRangeTooLarge  = errors.New("自定义日期范围不能超过 92 天")
	ErrPlaybackProfileOverviewTooLarge = errors.New("查询结果过大，请缩小时间范围或增加筛选条件")

	ErrInvalidDate    = ErrPlaybackHistoryInvalidDate
	ErrInvalidKeyword = ErrPlaybackHistoryInvalidKeyword
	ErrInvalidUserID  = ErrPlaybackHistoryInvalidUserID
	ErrUserNotFound   = ErrPlaybackHistoryUserNotFound
	ErrQueryFailed    = ErrPlaybackHistoryQueryFailed
)
