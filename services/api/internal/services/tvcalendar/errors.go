package tvcalendar

import "errors"

var (
	ErrTVCalendarInvalidDate       = errors.New("日期格式错误，应为 YYYY-MM-DD")
	ErrTVCalendarInvalidStatus     = errors.New("状态参数无效，仅支持 ready/missing/upcoming/today")
	ErrTVCalendarInvalidWeekOffset = errors.New("weekOffset 参数无效，仅支持 -1/0/1")
	ErrTVCalendarTMDBIDRequired    = errors.New("tmdbId 不能为空")
	ErrTVCalendarShowNameNeeded    = errors.New("showName 不能为空")
	ErrTVCalendarNotConfigured     = errors.New("TMDB API 未配置")

	ErrInvalidDate       = ErrTVCalendarInvalidDate
	ErrInvalidStatus     = ErrTVCalendarInvalidStatus
	ErrInvalidWeekOffset = ErrTVCalendarInvalidWeekOffset
	ErrTMDBIDRequired    = ErrTVCalendarTMDBIDRequired
	ErrShowNameNeeded    = ErrTVCalendarShowNameNeeded
	ErrNotConfigured     = ErrTVCalendarNotConfigured
)
