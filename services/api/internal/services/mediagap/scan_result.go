package mediagap

import (
	"context"
	"errors"
	"log"
)

// ScanFailure 只暴露扫描对象及安全原因，不向前端返回外部响应或连接详情。
type ScanFailure struct {
	TmdbID     string `json:"tmdbId"`
	SeriesName string `json:"seriesName"`
	Season     int    `json:"season,omitempty"`
	Reason     string `json:"reason"`
}

// Error 使季级失败可沿扫描调用栈保留安全的定位信息。
func (f *ScanFailure) Error() string { return f.Reason }

// scanSeriesBatch 汇总每剧结果；失败项可定向重试，取消后不再调用外部依赖。
func scanSeriesBatch(ctx context.Context, seriesItems []embySeriesItem, scan func(embySeriesItem) (*scanSeriesStats, error)) *ScanResult {
	result := &ScanResult{}
	for _, series := range seriesItems {
		var stats *scanSeriesStats
		err := ctx.Err()
		if err == nil {
			stats, err = scan(series)
		}
		if err != nil {
			result.SkippedSeries++
			failure := ScanFailure{TmdbID: extractProviderID(series.ProviderIDs, "Tmdb"), SeriesName: series.Name, Reason: "剧集扫描失败，请重试"}
			var detail *ScanFailure
			if errors.As(err, &detail) {
				failure = *detail
			}
			if ctx.Err() != nil {
				failure.Reason = "扫描已超时或取消"
			}
			result.Failures = append(result.Failures, failure)
			log.Printf("[MediaGap] 剧集扫描未完成 seriesId=%s tmdbId=%s season=%d reason=%q", series.ID, failure.TmdbID, failure.Season, failure.Reason)
			continue
		}
		result.ScannedSeries++
		result.ExaminedEpisodes += stats.Examined
		result.Created += stats.Created
		result.Updated += stats.Updated
		result.Ingested += stats.Ingested
	}
	return result
}
