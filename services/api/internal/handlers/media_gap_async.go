package handlers

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	mediagappkg "github.com/konghang/ember/backend/internal/services/mediagap"
)

const mediaGapAsyncScanTimeout = 30 * time.Minute

type mediaGapScanState string

const (
	mediaGapScanStateIdle      mediaGapScanState = "idle"
	mediaGapScanStateRunning   mediaGapScanState = "running"
	mediaGapScanStateSucceeded mediaGapScanState = "succeeded"
	mediaGapScanStateFailed    mediaGapScanState = "failed"
)

type mediaGapAsyncScanStatus struct {
	ScanID           string            `json:"scanId,omitempty"`
	Scope            string            `json:"scope,omitempty"`
	Status           mediaGapScanState `json:"status"`
	Running          bool              `json:"running"`
	StartedAt        *time.Time        `json:"startedAt,omitempty"`
	FinishedAt       *time.Time        `json:"finishedAt,omitempty"`
	Count            int               `json:"count"`
	ScannedSeries    int               `json:"scannedSeries"`
	SkippedSeries    int               `json:"skippedSeries"`
	ExaminedEpisodes int               `json:"examinedEpisodes"`
	Created          int               `json:"created"`
	Updated          int               `json:"updated"`
	Ingested         int               `json:"ingested"`
	Error            string            `json:"error,omitempty"`
	Message          string            `json:"message,omitempty"`
}

type mediaGapScanManager struct {
	mu     sync.RWMutex
	status mediaGapAsyncScanStatus
	scanFn func(context.Context, mediagappkg.ScanRequest) (*mediagappkg.ScanResult, error)
}

func newMediaGapScanManager(scanFn func(context.Context, mediagappkg.ScanRequest) (*mediagappkg.ScanResult, error)) *mediaGapScanManager {
	return &mediaGapScanManager{
		status: mediaGapAsyncScanStatus{
			Status:  mediaGapScanStateIdle,
			Message: "暂无扫描任务",
		},
		scanFn: scanFn,
	}
}

func (m *mediaGapScanManager) Start(req mediagappkg.ScanRequest) (mediaGapAsyncScanStatus, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.status.Running {
		current := m.status
		current.Message = "已有缺集扫描正在执行，请稍后查看状态"
		return current, false
	}

	now := time.Now().UTC()
	scanID := fmt.Sprintf("media-gap-scan-%d", now.UnixNano())
	scope := "all"
	if strings.TrimSpace(req.TMDBID) != "" {
		scope = "series"
	}

	m.status = mediaGapAsyncScanStatus{
		ScanID:    scanID,
		Scope:     scope,
		Status:    mediaGapScanStateRunning,
		Running:   true,
		StartedAt: cloneTimePointer(now),
		Message: func() string {
			if scope == "series" {
				return "单剧缺集扫描已启动，后台处理中"
			}
			return "全库缺集扫描已启动，后台处理中"
		}(),
	}

	go m.run(scanID, req)

	return m.status, true
}

func (m *mediaGapScanManager) Status() mediaGapAsyncScanStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

func (m *mediaGapScanManager) run(scanID string, req mediagappkg.ScanRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), mediaGapAsyncScanTimeout)
	defer cancel()

	result, err := m.scanFn(ctx, req)
	finishedAt := time.Now().UTC()

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.status.ScanID != scanID {
		return
	}

	m.status.Running = false
	m.status.FinishedAt = cloneTimePointer(finishedAt)

	if err != nil {
		m.status.Status = mediaGapScanStateFailed
		m.status.Error = err.Error()
		m.status.Message = "缺集扫描失败"
		return
	}

	if result != nil {
		m.status.Count = result.Created + result.Updated + result.Ingested
		m.status.ScannedSeries = result.ScannedSeries
		m.status.SkippedSeries = result.SkippedSeries
		m.status.ExaminedEpisodes = result.ExaminedEpisodes
		m.status.Created = result.Created
		m.status.Updated = result.Updated
		m.status.Ingested = result.Ingested
	}
	m.status.Error = ""
	m.status.Status = mediaGapScanStateSucceeded
	m.status.Message = "缺集扫描完成"
}

func cloneTimePointer(value time.Time) *time.Time {
	cloned := value
	return &cloned
}
