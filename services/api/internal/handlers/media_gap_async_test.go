package handlers

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	mediagappkg "github.com/konghang/ember/backend/internal/services/mediagap"
)

func TestMediaGapScanManagerStartAndComplete(t *testing.T) {
	done := make(chan struct{})
	manager := newMediaGapScanManager(func(ctx context.Context, req mediagappkg.ScanRequest) (*mediagappkg.ScanResult, error) {
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
	})

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
	manager := newMediaGapScanManager(func(ctx context.Context, req mediagappkg.ScanRequest) (*mediagappkg.ScanResult, error) {
		<-block
		return &mediagappkg.ScanResult{}, nil
	})

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
}

func TestMediaGapScanManagerMarksFailure(t *testing.T) {
	done := make(chan struct{})
	manager := newMediaGapScanManager(func(ctx context.Context, req mediagappkg.ScanRequest) (*mediagappkg.ScanResult, error) {
		defer close(done)
		return nil, errors.New("scan failed")
	})

	_, started := manager.Start(mediagappkg.ScanRequest{})
	if !started {
		t.Fatal("expected scan to start")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for failed async scan")
	}

	status := manager.Status()
	if status.Status != mediaGapScanStateFailed {
		t.Fatalf("expected failed status, got %s", status.Status)
	}
	if status.Error != "scan failed" {
		t.Fatalf("expected error message to be preserved, got %q", status.Error)
	}
}
