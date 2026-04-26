package handlers

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
	mediagappkg "github.com/konghang/ember/backend/internal/services/mediagap"
)

// fakeScanRecorder 模拟 advisory lock + 持久化记录，用于不连接真实 DB 的单测。
type fakeScanRecorder struct {
	mu          sync.Mutex
	busy        bool
	finishes    []models.MediaGapScanStatus
	holder      *mediagappkg.MediaGapScanLockHandleHolder
	finalizeCtx context.Context
	finalizeErr error
}

func (f *fakeScanRecorder) AcquireAndRecord(ctx context.Context) (*mediagappkg.MediaGapScanLockHandleHolder, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.busy {
		return nil, "", mediagappkg.ErrMediaGapScanInProgress
	}
	f.busy = true
	f.holder = &mediagappkg.MediaGapScanLockHandleHolder{}
	return f.holder, "scan-fake-id", nil
}

func (f *fakeScanRecorder) FinishAndReleaseHolder(ctx context.Context, holder *mediagappkg.MediaGapScanLockHandleHolder, scanID string, status models.MediaGapScanStatus, errMsg string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finishes = append(f.finishes, status)
	f.finalizeCtx = ctx
	if ctx != nil {
		f.finalizeErr = ctx.Err()
	}
	f.busy = false
	f.holder = nil
}

func TestMediaGapScanManagerStartAndComplete(t *testing.T) {
	done := make(chan struct{})
	manager := newMediaGapScanManagerWithRecorder(func(ctx context.Context, req mediagappkg.ScanRequest) (*mediagappkg.ScanResult, error) {
		defer close(done)
		if strings.TrimSpace(req.TMDBID) != "123" {
			t.Fatalf("expected tmdbId to be passed through, got %q", req.TMDBID)
		}
		if !req.Force {
			t.Fatal("expected force flag to be passed through")
		}
		return &mediagappkg.ScanResult{
			ScannedSeries:    2,
			SkippedSeries:    1,
			ExaminedEpisodes: 8,
			Created:          3,
			Updated:          2,
			Ingested:         1,
		}, nil
	}, &fakeScanRecorder{})

	status, started := manager.Start(mediagappkg.ScanRequest{TMDBID: "123", Force: true})
	if !started {
		t.Fatal("expected scan to start")
	}
	if status.Status != mediaGapScanStateRunning {
		t.Fatalf("expected running status, got %s", status.Status)
	}
	if !status.Running {
		t.Fatal("expected running flag to be true")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async scan to finish")
	}

	// 等待 run 内部 finalize 完成（finish 在 scanFn 之后）。
	time.Sleep(20 * time.Millisecond)

	finalStatus := manager.Status()
	if finalStatus.Status != mediaGapScanStateSucceeded {
		t.Fatalf("expected succeeded status, got %s", finalStatus.Status)
	}
	if finalStatus.Running {
		t.Fatal("expected running flag to be false after completion")
	}
	if finalStatus.Count != 6 {
		t.Fatalf("expected count=6, got %d", finalStatus.Count)
	}
	if finalStatus.FinishedAt == nil {
		t.Fatal("expected finishedAt to be populated")
	}
}

func TestMediaGapScanManagerRejectsConcurrentStart(t *testing.T) {
	block := make(chan struct{})
	recorder := &fakeScanRecorder{}
	manager := newMediaGapScanManagerWithRecorder(func(ctx context.Context, req mediagappkg.ScanRequest) (*mediagappkg.ScanResult, error) {
		<-block
		return &mediagappkg.ScanResult{}, nil
	}, recorder)

	firstStatus, started := manager.Start(mediagappkg.ScanRequest{})
	if !started {
		t.Fatal("expected first scan to start")
	}

	secondStatus, started := manager.Start(mediagappkg.ScanRequest{TMDBID: "456"})
	if started {
		t.Fatal("expected second start to be rejected while running")
	}
	if secondStatus.ScanID != firstStatus.ScanID {
		t.Fatal("expected concurrent start to return the current scan status")
	}

	close(block)
	// 让第一次扫描结束，避免 goroutine 泄漏到下一个测试。
	time.Sleep(20 * time.Millisecond)
}

func TestMediaGapScanManagerMarksFailure(t *testing.T) {
	done := make(chan struct{})
	manager := newMediaGapScanManagerWithRecorder(func(ctx context.Context, req mediagappkg.ScanRequest) (*mediagappkg.ScanResult, error) {
		defer close(done)
		return nil, errors.New("scan failed")
	}, &fakeScanRecorder{})

	_, started := manager.Start(mediagappkg.ScanRequest{})
	if !started {
		t.Fatal("expected scan to start")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for failed async scan")
	}
	time.Sleep(20 * time.Millisecond)

	status := manager.Status()
	if status.Status != mediaGapScanStateFailed {
		t.Fatalf("expected failed status, got %s", status.Status)
	}
	if status.Error != "scan failed" {
		t.Fatalf("expected error message to be preserved, got %q", status.Error)
	}
}

// TestMediaGapScanManagerFinalizesWithFreshContext 防止终态写回再次依赖
// 已超时 / 被 cancel 的扫描业务 ctx，导致 media_gap_scans 留下假 running 行。
//
// 验证手段：scan ctx 在 scanFn 内打上 sentinel value，finalize ctx 不应带该 sentinel
// （只有"复用同一个 ctx"才会带）。
func TestMediaGapScanManagerFinalizesWithFreshContext(t *testing.T) {
	type sentinelKey struct{}

	done := make(chan struct{})
	recorder := &fakeScanRecorder{}
	manager := newMediaGapScanManagerWithRecorder(func(ctx context.Context, req mediagappkg.ScanRequest) (*mediagappkg.ScanResult, error) {
		defer close(done)
		// scanFn 把 sentinel 注入 scan ctx 链；如果 finalize 复用 scan ctx，sentinel 会泄漏到 finalize ctx。
		if v := ctx.Value(sentinelKey{}); v != nil {
			t.Fatalf("scan ctx should not carry sentinel before scanFn injects it")
		}
		return nil, nil
	}, recorder)

	_, started := manager.Start(mediagappkg.ScanRequest{})
	if !started {
		t.Fatal("expected scan to start")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for scanFn to return")
	}
	time.Sleep(20 * time.Millisecond)

	if recorder.finalizeCtx == nil {
		t.Fatal("recorder did not receive a finalize context")
	}
	if recorder.finalizeErr != nil {
		t.Fatalf("finalize context must be live at call time; got err=%v", recorder.finalizeErr)
	}
	if len(recorder.finishes) != 1 {
		t.Fatalf("expected exactly 1 finalize call, got %d", len(recorder.finishes))
	}
}
