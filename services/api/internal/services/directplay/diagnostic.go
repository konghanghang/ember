package directplay

import (
	"context"
	"slices"
	"time"
)

type stepObserverKey struct{}
type stepObserver func(string, string, int64)

// WithStepObserver attaches request-local diagnostics without binding the
// service to Gateway logging or changing global logging configuration.
func WithStepObserver(ctx context.Context, observer func(step, phase string, elapsedMs int64)) context.Context {
	return context.WithValue(ctx, stepObserverKey{}, stepObserver(observer))
}

// observeStep reports fixed-name begin/end events. Finished means the call
// returned, not that it succeeded; the existing final decision owns outcomes.
func observeStep(ctx context.Context, name string) func() {
	if !slices.Contains(timingStages[:], name) && !slices.Contains([]string{"sourceLocation", "sessionWait", "mediaWait", "downloadWait", "routing", "leaseAdmission", "accounts", "leaseConfirm", "accessTouch", "healthUpdate", "transferAdmission", "taskBegin"}, name) {
		return func() {}
	}
	observer, _ := ctx.Value(stepObserverKey{}).(stepObserver)
	if observer == nil {
		return func() {}
	}
	started := time.Now()
	observer(name, "started", 0)
	return func() { observer(name, "finished", time.Since(started).Milliseconds()) }
}

// observeLeaseAdmission exposes only the allocation outcome, never Redis keys.
func observeLeaseAdmission(ctx context.Context, created bool) {
	observer, _ := ctx.Value(stepObserverKey{}).(stepObserver)
	if observer == nil {
		return
	}
	phase := "reused"
	if created {
		phase = "created"
	}
	observer("leaseAdmission", phase, 0)
}

// observeCache exposes a fixed outcome before later database/fallback work.
func observeCache(ctx context.Context, name, outcome string) {
	if outcome != "hit" && outcome != "miss" && outcome != "bypass" {
		return
	}
	if name != "downloadURLCache" && name != "mediaResolutionCache" {
		return
	}
	if observer, _ := ctx.Value(stepObserverKey{}).(stepObserver); observer != nil {
		observer(name, outcome, 0)
	}
}
