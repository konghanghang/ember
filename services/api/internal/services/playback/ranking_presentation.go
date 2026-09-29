package playback

import "time"

// RankingDisplayEnd 将零点排他上界转换为最后一个覆盖日期；周期内截点和零值原样保留。
// 该转换只用于显示，不改变 SQL 的自然日/周查询范围。
func RankingDisplayEnd(start, end time.Time, location *time.Location) time.Time {
	if end.IsZero() {
		return end
	}
	end = end.In(location)
	if end.After(start) && end.Hour() == 0 && end.Minute() == 0 && end.Second() == 0 && end.Nanosecond() == 0 {
		return end.Add(-time.Nanosecond)
	}
	return end
}
