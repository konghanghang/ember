package directplay

import (
	"context"
	"strconv"
	"time"
)

var timingStages = [...]string{"prepare", "sourceResolve", "targetSearch", "lockWait", "preID", "rapidUpload", "challenge", "targetVerify", "transferCommit", "downloadURL"}

type stageTiming struct {
	duration time.Duration
	calls    int
}

// TimingDiagnostics carries only request-local numeric measurements, never
// serialized or persisted. Stage names are filtered before logging.
type TimingDiagnostics struct {
	mediaCache    string
	downloadCache string
	total         time.Duration
	stages        map[string]stageTiming
}

// LogFields emits measured stages in fixed order, aggregating repeated calls.
// Missing stages are omitted; otherMs covers work outside measured operations.
func (timing TimingDiagnostics) LogFields() []string {
	if timing.stages == nil {
		return nil
	}
	fields := []string{"directPlayMs=" + strconv.FormatInt(timing.total.Milliseconds(), 10)}
	switch timing.mediaCache {
	case "hit", "miss", "bypass":
		fields = append(fields, "mediaResolutionCache="+timing.mediaCache)
	}
	switch timing.downloadCache {
	case "hit", "miss", "bypass":
		fields = append(fields, "downloadURLCache="+timing.downloadCache)
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
	mediaCache    string
	downloadCache string
	started       time.Time
	now           func() time.Time
	prepared      bool
	stages        map[string]stageTiming
}

// withTiming creates one recorder per resolve; shared Provider goroutines do
// not inherit it, so sourceResolve measures this caller's own wait.
func withTiming(ctx context.Context) (context.Context, *timingRecorder) {
	r := &timingRecorder{started: time.Now(), now: time.Now, stages: make(map[string]stageTiming)}
	return context.WithValue(ctx, timingContextKey{}, r), r
}

// finish includes early preparation failures as well as successful resolutions.
func (r *timingRecorder) finish() TimingDiagnostics {
	if !r.prepared {
		r.stages["prepare"] = stageTiming{duration: r.now().Sub(r.started), calls: 1}
	}
	return TimingDiagnostics{total: r.now().Sub(r.started), stages: r.stages, downloadCache: r.downloadCache, mediaCache: r.mediaCache}
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
	r, ok := ctx.Value(timingContextKey{}).(*timingRecorder)
	if !ok {
		return func() {}
	}
	started := r.now()
	return func() {
		stage := r.stages[name]
		stage.duration += r.now().Sub(started)
		stage.calls++
		r.stages[name] = stage
	}
}

// recordDownloadCache adds only a fixed cache outcome to the existing decision
// diagnostics; cache keys and download URLs never enter the recorder.
func recordDownloadCache(ctx context.Context, outcome string) {
	if r, ok := ctx.Value(timingContextKey{}).(*timingRecorder); ok {
		r.downloadCache = outcome
	}
}

// recordMediaCache records a fixed outcome only, never paths or credential keys.
func recordMediaCache(ctx context.Context, outcome string) {
	if r, ok := ctx.Value(timingContextKey{}).(*timingRecorder); ok {
		r.mediaCache = outcome
	}
}
