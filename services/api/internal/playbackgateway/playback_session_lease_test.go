package playbackgateway

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/konghang/ember/backend/internal/services/directplay"
)

type fakePlaybackSessionService struct {
	mu     sync.Mutex
	events []directplay.PlaybackSessionEvent
	result directplay.PlaybackSessionEventResult
	err    error
}

func (service *fakePlaybackSessionService) HandlePlaybackSessionEvent(
	_ context.Context,
	event directplay.PlaybackSessionEvent,
) (directplay.PlaybackSessionEventResult, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.events = append(service.events, event)
	return service.result, service.err
}

func (service *fakePlaybackSessionService) snapshot() []directplay.PlaybackSessionEvent {
	service.mu.Lock()
	defer service.mu.Unlock()
	return append([]directplay.PlaybackSessionEvent(nil), service.events...)
}

func TestGatewayUpdatesPlaybackLeaseOnlyAfterSuccessfulEmbyEvent(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/emby/Sessions/Playing/Progress" {
			t.Fatalf("unexpected upstream path %s", request.URL.Path)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	var logs bytes.Buffer
	gateway := newVideoTestGateway(t, upstream.URL, &fakeTokenService{principal: fixturePrincipal()}, nil, nil, &logs)
	leases := &fakePlaybackSessionService{result: directplay.PlaybackSessionEventResult{Found: true}}
	gateway.playbackSessionService = leases
	request := httptest.NewRequest(http.MethodPost, "/Sessions/Playing/Progress", bytes.NewBufferString(
		`{"ItemId":"item-1","MediaSourceId":"source-1","PlaySessionId":"session-1","PositionTicks":42,"IsPaused":true}`,
	))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(accessTokenHeader, fixtureAccessToken)
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("response = %d", response.Code)
	}
	events := leases.snapshot()
	if len(events) != 1 || events[0].UserID != "user-1" || events[0].MappingID != "mapping-1" ||
		events[0].DeviceID != "device-1" || events[0].PlaySessionID != "session-1" || !events[0].IsProgress || !events[0].IsPaused || events[0].Stopped {
		t.Fatalf("events = %+v", events)
	}
}

func TestGatewayDoesNotUpdatePlaybackLeaseAfterFailedEmbyEvent(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadGateway)
	}))
	defer upstream.Close()

	gateway := newVideoTestGateway(t, upstream.URL, &fakeTokenService{principal: fixturePrincipal()}, nil, nil, &bytes.Buffer{})
	leases := &fakePlaybackSessionService{}
	gateway.playbackSessionService = leases
	request := httptest.NewRequest(http.MethodPost, "/Sessions/Playing/Stopped", bytes.NewBufferString(
		`{"ItemId":"item-1","MediaSourceId":"source-1","PlaySessionId":"session-1"}`,
	))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(accessTokenHeader, fixtureAccessToken)
	gateway.ServeHTTP(httptest.NewRecorder(), request)

	if events := leases.snapshot(); len(events) != 0 {
		t.Fatalf("failed upstream updated leases: %+v", events)
	}
}

func TestGatewayLeaseUpdateFailureDoesNotChangeSuccessfulEmbyResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	var logs bytes.Buffer
	gateway := newVideoTestGateway(t, upstream.URL, &fakeTokenService{principal: fixturePrincipal()}, nil, nil, &logs)
	gateway.logger = log.New(&logs, "", 0)
	gateway.playbackSessionService = &fakePlaybackSessionService{err: directplay.ErrRedisUnavailable}
	request := httptest.NewRequest(http.MethodPost, "/Sessions/Playing", bytes.NewBufferString(
		`{"ItemId":"item-1","MediaSourceId":"source-1","PlaySessionId":"session-1"}`,
	))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(accessTokenHeader, fixtureAccessToken)
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent || !bytes.Contains(logs.Bytes(), []byte("reasonCode=redis_unavailable")) {
		t.Fatalf("response=%d logs=%q", response.Code, logs.String())
	}
}
