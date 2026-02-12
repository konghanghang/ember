package common

import "time"

// CalculateExpiryDate 计算到期日期
// days: 有效天数
// 返回：从当前时间开始 N 天后的日期
func CalculateExpiryDate(days int) time.Time {
	return time.Now().UTC().AddDate(0, 0, days)
}
