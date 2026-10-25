package directplay

import (
	"context"
	"strconv"
	"time"
)

var timingStages = [...]string{"prepare", "sourceResolve", "targetSearch", "lockWait", "preIDWait", "preID", "rapidUpload", "challenge", "targetVerify", "transferCommit", "downloadURL"}

type stageTiming struct {
	duration time.Duration
	calls    int
}

// TimingDiagnostics carries request timings, fixed cache outcomes and optional
// process-local eviction snapshots. Names and reasons are filtered before logging.
type TimingDiagnostics struct {
	preIDCache                                  string
	mediaCache, downloadCache                   string
	mediaCacheReason, downloadCacheReason       string
	mediaCacheEvictions, downloadCacheEvictions uint64
	mediaCacheObserved, downloadCacheObserved   bool
	total                                       time.Duration
	stages                                      map[string]stageTiming
}

// LogFields emits measured stages in fixed order, aggregating repeated calls.
// Missing stages are omitted; otherMs covers work outside measured operations.
func (timing TimingDiagnostics) LogFields() []string {
	if timing.stages == nil {
		return nil
	}
	fields := []string{"directPlayMs=" + strconv.FormatInt(timing.total.Milliseconds(), 10)}
	switch timing.preIDCache {
	case "hit", "miss", "bypass":
		fields = append(fields, "preIDCache="+timing.preIDCache)
	}
	switch timing.mediaCache {
	case "hit", "miss", "bypass":
		fields = append(fields, "mediaResolutionCache="+timing.mediaCache)
	}
	if timing.mediaCache == "miss" || timing.mediaCache == "bypass" {
		switch timing.mediaCacheReason {
		case "entry_missing", "expired", "clock_rollback", "freshness_expired",
			"dependent_url_missing", "dependent_url_expired", "dependent_url_clock_rollback", "dependent_url_host_rejected",
			"key_unavailable", "cache_unavailable":
			fields = append(fields, "mediaResolutionCacheReason="+timing.mediaCacheReason)
		}
	}
	switch timing.downloadCache {
	case "hit", "miss", "bypass":
		fields = append(fields, "downloadURLCache="+timing.downloadCache)
	}
	if timing.downloadCache == "miss" || timing.downloadCache == "bypass" {
		switch timing.downloadCacheReason {
		case "entry_missing", "expired", "clock_rollback", "host_rejected", "key_unavailable", "cache_unavailable", "forced_refresh":
			fields = append(fields, "downloadURLCacheReason="+timing.downloadCacheReason)
		}
	}
	if timing.mediaCacheObserved {
		fields = append(fields, "mediaCacheEvictions="+strconv.FormatUint(timing.mediaCacheEvictions, 10))
	}
	if timing.downloadCacheObserved {
		fields = append(fields, "downloadCacheEvictions="+strconv.FormatUint(timing.downloadCacheEvictions, 10))
	}
	var measured time.Duration
	for _, name := range timingStages {
		stage, found := timing.stages[name]
		if !found {
			continue
		}
		measured += stage.duration
		fields = append(fields, name+"Ms="+strconv.FormatInt(stage.duration.Milliseconds(), 10), name+"Calls="+strconv.Itoa(stage.calls))
	}
	other := timing.total - measured
	if other < 0 {
		other = 0
	}
	return append(fields, "otherMs="+strconv.FormatInt(other.Milliseconds(), 10))
}

type timingContextKey struct{}
type timingRecorder struct {
	preIDCache                            string
	mediaCache, downloadCache             string
	mediaCacheReason, downloadCacheReason string
	mediaCacheSource                      *mediaResolutionCache
	downloadCacheSource                   *downloadURLCache
	started                               time.Time
	now                                   func() time.Time
	prepared                              bool
	stages                                map[string]stageTiming
}

// withTiming creates one recorder per resolve; shared Provider goroutines do
// not inherit it, so sourceResolve measures this caller's own wait.
func withTiming(ctx context.Context) (context.Context, *timingRecorder) {
	r := &timingRecorder{started: time.Now(), now: time.Now, stages: make(map[string]stageTiming)}
	return context.WithValue(ctx, timingContextKey{}, r), r
}

// finish includes early failures and snapshots each observed cache under its
// existing mutex. Evictions are process totals, not this request's increments.
func (r *timingRecorder) finish() TimingDiagnostics {
	if !r.prepared {
		r.stages["prepare"] = stageTiming{duration: r.now().Sub(r.started), calls: 1}
	}
	timing := TimingDiagnostics{total: r.now().Sub(r.started), stages: r.stages,
		downloadCache: r.downloadCache, mediaCache: r.mediaCache, preIDCache: r.preIDCache,
		downloadCacheReason: r.downloadCacheReason, mediaCacheReason: r.mediaCacheReason}
	if r.mediaCacheSource != nil {
		timing.mediaCacheObserved = true
		timing.mediaCacheEvictions = r.mediaCacheSource.evictionCount()
	}
	if r.downloadCacheSource != nil {
		timing.downloadCacheObserved = true
		timing.downloadCacheEvictions = r.downloadCacheSource.evictionCount()
	}
	return timing
}

// finishPreparation separates mapping, account loading and Redis admission
// from Provider operations without nesting measured spans.
func finishPreparation(ctx context.Context) {
	if r, ok := ctx.Value(timingContextKey{}).(*timingRecorder); ok && !r.prepared {
		r.stages["prepare"] = stageTiming{duration: r.now().Sub(r.started), calls: 1}
		r.prepared = true
	}
}

// measureStage records a synchronous operation including its failing return.
// The monotonic clock is independent of the configured business timezone.
func measureStage(ctx context.Context, name string) func() {
	finishDiagnostic := observeStep(ctx, name)
	r, ok := ctx.Value(timingContextKey{}).(*timingRecorder)
	if !ok {
		return finishDiagnostic
	}
	started := r.now()
	return func() {
		finishDiagnostic()
		stage := r.stages[name]
		stage.duration += r.now().Sub(started)
		stage.calls++
		r.stages[name] = stage
	}
}

// recordDownloadCache adds a fixed outcome and reason to decision diagnostics;
// the Debug observer retains its original outcome-only contract.
func recordDownloadCache(ctx context.Context, outcome, reason string) {
	observeCache(ctx, "downloadURLCache", outcome)
	if r, ok := ctx.Value(timingContextKey{}).(*timingRecorder); ok {
		r.downloadCache = outcome
		r.downloadCacheReason = reason
	}
}

// recordMediaCache records fixed evidence, never paths or credential keys.
// Hits clear a prior miss reason before the final decision is emitted.
func recordMediaCache(ctx context.Context, outcome, reason string) {
	observeCache(ctx, "mediaResolutionCache", outcome)
	if r, ok := ctx.Value(timingContextKey{}).(*timingRecorder); ok {
		r.mediaCache = outcome
		r.mediaCacheReason = reason
	}
}

// recordPreIDCache emits only a fixed local outcome, never another request's
// Provider measurements, source identity digest or validated hash.
func recordPreIDCache(ctx context.Context, outcome string) {
	observeCache(ctx, "preIDCache", outcome)
	if r, ok := ctx.Value(timingContextKey{}).(*timingRecorder); ok {
		r.preIDCache = outcome
	}
}

// recordCacheUse registers only caches actually consulted by this request.
// finish samples after later cache publication without retaining keys or URLs.
func recordCacheUse(ctx context.Context, media *mediaResolutionCache, download *downloadURLCache) {
	if r, ok := ctx.Value(timingContextKey{}).(*timingRecorder); ok {
		if media != nil {
			r.mediaCacheSource = media
		}
		if download != nil {
			r.downloadCacheSource = download
		}
	}
}
