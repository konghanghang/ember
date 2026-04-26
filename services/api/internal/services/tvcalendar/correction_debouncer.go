package tvcalendar

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/konghang/ember/backend/internal/async"
	"github.com/konghang/ember/backend/internal/db"
)

// correctionDebouncer 进程内 channel + ticker 按 seriesId 去重 buffer
// 30s 窗口内同 seriesId 只触发一次持久化写入
type correctionDebouncer struct {
	ch       chan string
	mu       sync.Mutex
	pending  map[string]struct{}
	interval time.Duration
}

var defaultDebouncer = newCorrectionDebouncer(30 * time.Second)

func newCorrectionDebouncer(interval time.Duration) *correctionDebouncer {
	d := &correctionDebouncer{
		ch:       make(chan string, 200),
		pending:  make(map[string]struct{}),
		interval: interval,
	}
	async.SafeGo("tv_calendar.correction.flush", func() {
		d.run(context.Background())
	})
	return d
}

func (d *correctionDebouncer) Push(seriesID string) {
	select {
	case d.ch <- seriesID:
	default:
		// channel 满时直接丢弃（写操作低频，不阻塞 GET 响应）
	}
}

func (d *correctionDebouncer) run(ctx context.Context) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()
	for {
		select {
		case id := <-d.ch:
			d.mu.Lock()
			d.pending[id] = struct{}{}
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
	ids := make([]string, 0, len(d.pending))
	for id := range d.pending {
		ids = append(ids, id)
	}
	d.pending = make(map[string]struct{})
	d.mu.Unlock()

	if len(ids) == 0 {
		return
	}

	// 批量更新 lastCorrectionAt
	result := db.DB.WithContext(ctx).Exec(
		`UPDATE tv_calendar_sources SET "lastCorrectionAt" = now() WHERE "seriesId" = ANY(?)`, ids)
	if result.Error != nil {
		log.Printf("[TVCalendar] correctionDebouncer flush 失败（%d 个 seriesId）：%v", len(ids), result.Error)
	} else {
		log.Printf("[TVCalendar] correctionDebouncer flush：更新 %d 个 seriesId 的 lastCorrectionAt", result.RowsAffected)
	}
}
