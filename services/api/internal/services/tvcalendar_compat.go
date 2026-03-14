package services

import (
	"time"

	tvcalendarpkg "github.com/konghang/ember/backend/internal/services/tvcalendar"
)

type TVCalendarService = tvcalendarpkg.TVCalendarService
type TVCalendarDTO = tvcalendarpkg.TVCalendarDTO
type TVCalendarWeeklyDTO = tvcalendarpkg.TVCalendarWeeklyDTO
type TVCalendarDayDTO = tvcalendarpkg.TVCalendarDayDTO
type TVCalendarWeeklyItem = tvcalendarpkg.TVCalendarWeeklyItem
type CreateTVCalendarSubscriptionRequest = tvcalendarpkg.CreateTVCalendarSubscriptionRequest

var (
	ErrTVCalendarInvalidDate       = tvcalendarpkg.ErrInvalidDate
	ErrTVCalendarInvalidStatus     = tvcalendarpkg.ErrInvalidStatus
	ErrTVCalendarInvalidWeekOffset = tvcalendarpkg.ErrInvalidWeekOffset
	ErrTVCalendarTMDBIDRequired    = tvcalendarpkg.ErrTMDBIDRequired
	ErrTVCalendarShowNameNeeded    = tvcalendarpkg.ErrShowNameNeeded
	ErrTVCalendarNotConfigured     = tvcalendarpkg.ErrNotConfigured
)

func NewTVCalendarService() *TVCalendarService {
	return tvcalendarpkg.NewTVCalendarService()
}

func DefaultTVCalendarWeekOffsets() []int {
	return tvcalendarpkg.DefaultTVCalendarWeekOffsets()
}

func ParseTVCalendarWeekOffset(value string) (int, error) {
	return tvcalendarpkg.ParseTVCalendarWeekOffset(value)
}

func ParseTVCalendarWeekDate(weekDateValue, weekOffsetValue string) (time.Time, error) {
	return tvcalendarpkg.ParseTVCalendarWeekDate(weekDateValue, weekOffsetValue)
}

func ParseTVCalendarDate(value string) (time.Time, error) {
	return tvcalendarpkg.ParseTVCalendarDate(value)
}

func DefaultTVCalendarDateRange() (time.Time, time.Time) {
	return tvcalendarpkg.DefaultTVCalendarDateRange()
}
