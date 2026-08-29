package playbackgateway

import (
	"bytes"
	"encoding/json"
	"hash/maphash"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultPlaybackSessionRequestMaxSize    = int64(64 << 10)
	defaultPlaybackSessionFailureMaxEntries = 4096
	defaultPlaybackSessionFailureTTL        = 6 * time.Hour
)

type playbackSessionEventKind string

const (
	playbackSessionEventNone     playbackSessionEventKind = ""
	playbackSessionEventStart    playbackSessionEventKind = "start"
	playbackSessionEventProgress playbackSessionEventKind = "progress"
	playbackSessionEventStop     playbackSessionEventKind = "stop"
)

type playbackSessionEventPayload struct {
	ItemID        string `json:"ItemId"`
	MediaSourceID string `json:"MediaSourceId"`
	PlaySessionID string `json:"PlaySessionId"`
	PositionTicks *int64 `json:"PositionTicks"`
	IsPaused      *bool  `json:"IsPaused"`
}

// playbackSessionEventSnapshot contains only bounded, non-secret playback
// identifiers needed to diagnose whether an Emby session event was forwarded.
type playbackSessionEventSnapshot struct {
	kind               playbackSessionEventKind
	itemID             string
	mediaSourceID      string
	playSessionID      string
	positionTicks      int64
	positionPresent    bool
	isPaused           bool
	isPausedPresent    bool
	snapshotState      string
	correlationKey     uint64
	correlationPresent bool
}

type playbackSessionFailureKey struct {
	correlationKey     uint64
	correlationPresent bool
	itemID             string
	mediaSourceID      string
	playSessionID      string
}

type playbackSessionFailureTracker struct {
	mu         sync.Mutex
	entries    map[playbackSessionFailureKey]time.Time
	maxEntries int
	ttl        time.Duration
	seed       maphash.Seed
}

// newPlaybackSessionFailureTracker creates a bounded in-memory observation
// cache used only to emit one recovery log after a failed session report.
func newPlaybackSessionFailureTracker(maxEntries int, ttl time.Duration) *playbackSessionFailureTracker {
	return &playbackSessionFailureTracker{
		entries: make(map[playbackSessionFailureKey]time.Time), maxEntries: maxEntries, ttl: ttl,
		seed: maphash.MakeSeed(),
	}
}

// newPlaybackSessionEventSnapshot classifies a session route without reading
// an unauthenticated request body.
func newPlaybackSessionEventSnapshot(request *http.Request) playbackSessionEventSnapshot {
	event := playbackSessionEventSnapshot{kind: playbackSessionEventKindForRequest(request)}
	if event.kind != playbackSessionEventNone {
		event.snapshotState = "not_inspected"
	}
	return event
}

// inspectPlaybackSessionRequest records a bounded JSON sidecar while restoring
// the exact original request bytes for the upstream Emby 4.9 endpoint.
func (gateway *Gateway) inspectPlaybackSessionRequest(request *http.Request) playbackSessionEventSnapshot {
	event := playbackSessionEventSnapshot{kind: playbackSessionEventKindForRequest(request)}
	if event.kind == playbackSessionEventNone {
		return event
	}
	event.snapshotState = "missing_body"
	if request == nil || request.Body == nil {
		return event
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		event.snapshotState = "content_type_invalid"
		return event
	}
	if encoding := strings.TrimSpace(request.Header.Get("Content-Encoding")); encoding != "" && !strings.EqualFold(encoding, "identity") {
		event.snapshotState = "content_encoding_unsupported"
		return event
	}

	originalBody := request.Body
	prefix, readErr := io.ReadAll(io.LimitReader(originalBody, gateway.maxPlaybackSessionRequestBytes+1))
	request.Body = &replayedBody{Reader: io.MultiReader(bytes.NewReader(prefix), originalBody), closer: originalBody}
	if readErr != nil {
		event.snapshotState = "read_failed"
		return event
	}
	if int64(len(prefix)) > gateway.maxPlaybackSessionRequestBytes {
		event.snapshotState = "too_large"
		return event
	}

	var payload playbackSessionEventPayload
	if err := json.Unmarshal(prefix, &payload); err != nil ||
		!validProofValue(payload.ItemID, maxProofItemIDBytes, false) ||
		!validProofValue(payload.MediaSourceID, maxProofMediaSourceIDBytes, true) ||
		!validProofValue(payload.PlaySessionID, maxProofPlaySessionIDBytes, false) ||
		(payload.PositionTicks != nil && *payload.PositionTicks < 0) {
		event.snapshotState = "invalid"
		return event
	}

	event.itemID = payload.ItemID
	event.mediaSourceID = payload.MediaSourceID
	event.playSessionID = payload.PlaySessionID
	if payload.PositionTicks != nil {
		event.positionTicks = *payload.PositionTicks
		event.positionPresent = true
	}
	if payload.IsPaused != nil {
		event.isPaused = *payload.IsPaused
		event.isPausedPresent = true
	}
	event.snapshotState = "recorded"
	return event
}

// logPlaybackSessionEvent emits readable session lifecycle diagnostics without
// changing the response or pretending a failed Emby forward succeeded.
func (gateway *Gateway) logPlaybackSessionEvent(
	event playbackSessionEventSnapshot,
	statusCode int,
	forwardAttempted bool,
	startedAt time.Time,
) {
	if gateway == nil || event.kind == playbackSessionEventNone {
		return
	}
	now := time.Now().UTC()
	durationMs := now.Sub(startedAt).Milliseconds()
	forwardState := "not_attempted"
	if forwardAttempted {
		forwardState = "attempted"
	}
	result := "failure"
	if statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices {
		result = "success"
	}

	if result == "failure" {
		gateway.playbackSessionFailures.Record(event, now)
		gateway.logger.Printf(
			"[PlaybackGateway] code=%s message=%q event=%s result=failure forwardState=%s statusCode=%d itemId=%q mediaSourceId=%q playSessionId=%q positionTicks=%d positionPresent=%t isPaused=%s snapshotState=%s durationMs=%d",
			playbackSessionFailureCode(event.kind), playbackSessionFailureMessage(event.kind), event.kind,
			forwardState, statusCode, event.itemID, event.mediaSourceID, event.playSessionID,
			event.positionTicks, event.positionPresent, event.pausedState(), event.snapshotState, durationMs,
		)
		return
	}

	if event.kind == playbackSessionEventStart || event.kind == playbackSessionEventProgress {
		if interruptionMs, recovered := gateway.playbackSessionFailures.Recover(event, now); recovered {
			gateway.logger.Printf(
				"[PlaybackGateway] code=playback_progress_recovered message=%q result=success recoveryEvent=%s itemId=%q playSessionId=%q positionTicks=%d positionPresent=%t isPaused=%s interruptionMs=%d",
				"播放进度上报已恢复", event.kind, event.itemID, event.playSessionID, event.positionTicks,
				event.positionPresent, event.pausedState(), interruptionMs,
			)
		}
	} else if event.kind == playbackSessionEventStop {
		gateway.playbackSessionFailures.Clear(event)
	}

	format := "[PlaybackGateway] code=%s message=%q event=%s result=success forwardState=%s statusCode=%d itemId=%q mediaSourceId=%q playSessionId=%q positionTicks=%d positionPresent=%t isPaused=%s snapshotState=%s durationMs=%d"
	args := []interface{}{
		playbackSessionSuccessCode(event.kind), playbackSessionSuccessMessage(event.kind), event.kind,
		forwardState, statusCode, event.itemID, event.mediaSourceID, event.playSessionID,
		event.positionTicks, event.positionPresent, event.pausedState(), event.snapshotState, durationMs,
	}
	if event.kind == playbackSessionEventProgress {
		gateway.debugf("[PlaybackGateway] level=debug "+strings.TrimPrefix(format, "[PlaybackGateway] "), args...)
		return
	}
	gateway.logger.Printf(format, args...)
}

// playbackSessionEventKindForRequest recognizes only the three versioned Emby
// session POST endpoints after root-path normalization.
func playbackSessionEventKindForRequest(request *http.Request) playbackSessionEventKind {
	if request == nil || request.URL == nil || request.Method != http.MethodPost {
		return playbackSessionEventNone
	}
	switch request.URL.Path {
	case "/emby/Sessions/Playing":
		return playbackSessionEventStart
	case "/emby/Sessions/Playing/Progress":
		return playbackSessionEventProgress
	case "/emby/Sessions/Playing/Stopped":
		return playbackSessionEventStop
	default:
		return playbackSessionEventNone
	}
}

// pausedState preserves the distinction between false and an absent IsPaused.
func (event playbackSessionEventSnapshot) pausedState() string {
	if !event.isPausedPresent {
		return "unknown"
	}
	if event.isPaused {
		return "true"
	}
	return "false"
}

// failureKey prevents unrelated items or media sources that reuse a client
// PlaySessionId from producing a false recovery observation.
func (event playbackSessionEventSnapshot) failureKey() playbackSessionFailureKey {
	if event.correlationPresent {
		return playbackSessionFailureKey{
			correlationKey: event.correlationKey, correlationPresent: true,
		}
	}
	return playbackSessionFailureKey{
		itemID: event.itemID, mediaSourceID: event.mediaSourceID, playSessionID: event.playSessionID,
	}
}

// playbackSessionSuccessCode maps a fixed event kind to a searchable log code.
func playbackSessionSuccessCode(kind playbackSessionEventKind) string {
	switch kind {
	case playbackSessionEventStart:
		return "playback_session_started"
	case playbackSessionEventProgress:
		return "playback_progress_reported"
	case playbackSessionEventStop:
		return "playback_session_stopped"
	default:
		return "playback_session_reported"
	}
}

// playbackSessionFailureCode maps a fixed event kind to a searchable log code.
func playbackSessionFailureCode(kind playbackSessionEventKind) string {
	switch kind {
	case playbackSessionEventStart:
		return "playback_session_start_failed"
	case playbackSessionEventProgress:
		return "playback_progress_failed"
	case playbackSessionEventStop:
		return "playback_session_stop_failed"
	default:
		return "playback_session_report_failed"
	}
}

// playbackSessionSuccessMessage supplies the human-readable Chinese outcome.
func playbackSessionSuccessMessage(kind playbackSessionEventKind) string {
	switch kind {
	case playbackSessionEventStart:
		return "播放开始上报成功"
	case playbackSessionEventProgress:
		return "播放进度上报成功"
	case playbackSessionEventStop:
		return "播放停止上报成功"
	default:
		return "播放会话上报成功"
	}
}

// playbackSessionFailureMessage supplies the human-readable Chinese failure.
func playbackSessionFailureMessage(kind playbackSessionEventKind) string {
	switch kind {
	case playbackSessionEventStart:
		return "播放开始上报失败"
	case playbackSessionEventProgress:
		return "播放进度上报失败"
	case playbackSessionEventStop:
		return "播放停止上报失败"
	default:
		return "播放会话上报失败"
	}
}

// Record remembers the first failure for a bounded valid PlaySessionId.
func (tracker *playbackSessionFailureTracker) Record(event playbackSessionEventSnapshot, at time.Time) {
	if tracker == nil || tracker.maxEntries <= 0 || tracker.ttl <= 0 ||
		(event.snapshotState != "recorded" && !event.correlationPresent) {
		return
	}
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	tracker.pruneLocked(at)
	key := event.failureKey()
	if _, exists := tracker.entries[key]; exists {
		return
	}
	if len(tracker.entries) >= tracker.maxEntries {
		tracker.evictOldestLocked()
	}
	tracker.entries[key] = at
}

// Recover removes one failed-session observation and reports its interruption.
func (tracker *playbackSessionFailureTracker) Recover(event playbackSessionEventSnapshot, at time.Time) (int64, bool) {
	if tracker == nil || (event.snapshotState != "recorded" && !event.correlationPresent) {
		return 0, false
	}
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	tracker.pruneLocked(at)
	key := event.failureKey()
	failedAt, ok := tracker.entries[key]
	if !ok {
		return 0, false
	}
	delete(tracker.entries, key)
	duration := at.Sub(failedAt)
	if duration < 0 {
		duration = 0
	}
	return duration.Milliseconds(), true
}

// Clear removes a stopped session without claiming that progress recovered.
func (tracker *playbackSessionFailureTracker) Clear(event playbackSessionEventSnapshot) {
	if tracker == nil || (event.snapshotState != "recorded" && !event.correlationPresent) {
		return
	}
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	delete(tracker.entries, event.failureKey())
}

// CorrelationKey returns a process-local, non-logged token correlation value
// so a local identity-store failure can match the next report without retaining
// the raw Token or a reusable stable digest.
func (tracker *playbackSessionFailureTracker) CorrelationKey(accessToken string) (uint64, bool) {
	if tracker == nil || accessToken == "" {
		return 0, false
	}
	return maphash.String(tracker.seed, accessToken), true
}

// pruneLocked removes observations that can no longer produce a recovery log.
func (tracker *playbackSessionFailureTracker) pruneLocked(now time.Time) {
	for key, failedAt := range tracker.entries {
		if !failedAt.Add(tracker.ttl).After(now) {
			delete(tracker.entries, key)
		}
	}
}

// evictOldestLocked bounds memory without requiring a background goroutine.
func (tracker *playbackSessionFailureTracker) evictOldestLocked() {
	var oldestKey playbackSessionFailureKey
	var oldest time.Time
	found := false
	for key, failedAt := range tracker.entries {
		if !found || failedAt.Before(oldest) {
			oldestKey = key
			oldest = failedAt
			found = true
		}
	}
	if found {
		delete(tracker.entries, oldestKey)
	}
}
