package tvcalendar

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/konghang/ember/backend/internal/async"
	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
)

// correctionDebouncer 进程内 channel + ticker 按 seriesId 去重 buffer
// 30s 窗口内同 seriesId 只触发一次持久化写入
type correctionDebouncer struct {
	ch       chan readyCorrection
	mu       sync.Mutex
	pending  map[string]readyCorrection
	interval time.Duration
}

var defaultDebouncer = newCorrectionDebouncer(30 * time.Second)

func newCorrectionDebouncer(interval time.Duration) *correctionDebouncer {
	d := &correctionDebouncer{
		ch:       make(chan readyCorrection, 200),
		pending:  make(map[string]readyCorrection),
		interval: interval,
	}
	async.SafeGo("tv_calendar.correction.flush", func() {
		d.run(context.Background())
	})
	return d
}

func buildReadyCorrectionKey(c readyCorrection) string {
	return c.TmdbID + ":" + buildEpisodeKey(c.Season, c.Episode)
}

func (d *correctionDebouncer) Push(correction readyCorrection) {
	select {
	case d.ch <- correction:
	default:
		// channel 满时直接丢弃（写操作低频，不阻塞 GET 响应）
	}
}

func (d *correctionDebouncer) run(ctx context.Context) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()
	for {
		select {
		case correction := <-d.ch:
			d.mu.Lock()
			d.pending[buildReadyCorrectionKey(correction)] = correction
			d.mu.Unlock()
		case <-ticker.C:
			d.flush(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (d *correctionDebouncer) flush(ctx context.Context) {
	d.mu.Lock()
	corrections := make([]readyCorrection, 0, len(d.pending))
	seriesIDs := make(map[string]struct{}, len(d.pending))
	for _, correction := range d.pending {
		corrections = append(corrections, correction)
		if correction.SeriesID != "" {
			seriesIDs[correction.SeriesID] = struct{}{}
		}
	}
	d.pending = make(map[string]readyCorrection)
	d.mu.Unlock()

	if len(corrections) == 0 {
		return
	}

	now := time.Now().UTC()
	updatedItems := int64(0)
	for _, correction := range corrections {
		updates := map[string]interface{}{
			"status":      models.TVCalendarStatusReady,
			"lastChecked": now,
		}
		if correction.EmbyItemID != "" {
			updates["embyItemId"] = correction.EmbyItemID
		}
		if correction.SeriesID != "" {
			updates["seriesId"] = correction.SeriesID
		}
		result := db.DB.WithContext(ctx).
			Model(&models.TVCalendarItem{}).
			Where(`"tmdbId" = ? AND season = ? AND episode = ?`, correction.TmdbID, correction.Season, correction.Episode).
			Updates(updates)
		if result.Error != nil {
			log.Printf("[TVCalendar] correctionDebouncer item flush 失败 tmdbId=%s season=%d episode=%d err=%v",
				correction.TmdbID, correction.Season, correction.Episode, result.Error)
			continue
		}
		updatedItems += result.RowsAffected
	}

	if len(seriesIDs) == 0 {
		log.Printf("[TVCalendar] correctionDebouncer flush：更新 %d 条 tv_calendar_items，无需回写 source marker", updatedItems)
		return
	}

	ids := make([]string, 0, len(seriesIDs))
	for seriesID := range seriesIDs {
		ids = append(ids, seriesID)
	}

	result := db.DB.WithContext(ctx).Exec(
		`UPDATE tv_calendar_sources SET "lastCorrectionAt" = now() WHERE "seriesId" = ANY(?)`, ids)
	if result.Error != nil {
		log.Printf("[TVCalendar] correctionDebouncer source flush 失败（%d 个 seriesId）：%v", len(ids), result.Error)
	} else {
		log.Printf("[TVCalendar] correctionDebouncer flush：更新 %d 条 tv_calendar_items，回写 %d 个 seriesId 的 lastCorrectionAt", updatedItems, result.RowsAffected)
	}
}
