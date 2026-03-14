package playback

import "errors"

var (
	ErrPlaybackHistoryInvalidDate    = errors.New("日期格式错误，应为 YYYY-MM-DD")
	ErrPlaybackHistoryInvalidKeyword = errors.New("keyword 仅支持中英文、数字、空格和 -_.'，且长度不能超过 100")
	ErrPlaybackHistoryInvalidUserID  = errors.New("userId 格式无效")
	ErrPlaybackHistoryUserNotFound   = errors.New("用户不存在")
	ErrPlaybackHistoryQueryFailed    = errors.New("Playback Reporting 查询失败")

	ErrInvalidDate    = ErrPlaybackHistoryInvalidDate
	ErrInvalidKeyword = ErrPlaybackHistoryInvalidKeyword
	ErrInvalidUserID  = ErrPlaybackHistoryInvalidUserID
	ErrUserNotFound   = ErrPlaybackHistoryUserNotFound
	ErrQueryFailed    = ErrPlaybackHistoryQueryFailed
)
