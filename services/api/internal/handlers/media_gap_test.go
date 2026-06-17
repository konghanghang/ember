package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
	mediagappkg "github.com/konghang/ember/backend/internal/services/mediagap"
)

func TestNormalizeDispatchPayload(t *testing.T) {
	candidatePayload := map[string]interface{}{
		"id":    "payload-id",
		"title": "Payload Title",
	}
	nestedPayload := map[string]interface{}{
		"guid": "nested-guid",
	}

	testCases := []struct {
		name string
		req  mediaGapDispatchRequest
		want map[string]interface{}
	}{
		{
			name: "candidate payload wins",
			req: mediaGapDispatchRequest{
				CandidateID:      " candidate-id ",
				CandidatePayload: candidatePayload,
				Candidate: map[string]interface{}{
					"payload": nestedPayload,
				},
			},
			want: candidatePayload,
		},
		{
			name: "nested candidate payload",
			req: mediaGapDispatchRequest{
				Candidate: map[string]interface{}{
					"title":   "Candidate Title",
					"payload": nestedPayload,
				},
			},
			want: nestedPayload,
		},
		{
			name: "candidate object fallback",
			req: mediaGapDispatchRequest{
				Candidate: map[string]interface{}{
					"title": "Candidate Title",
					"site":  "SiteA",
				},
			},
			want: map[string]interface{}{
				"title": "Candidate Title",
				"site":  "SiteA",
			},
		},
		{
			name: "candidate id fallback",
			req: mediaGapDispatchRequest{
				CandidateID: " candidate-id ",
			},
			want: map[string]interface{}{
				"id": "candidate-id",
			},
		},
		{
			name: "empty request",
			req:  mediaGapDispatchRequest{},
			want: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeDispatchPayload(tc.req)

			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("expected payload %+v, got %+v", tc.want, got)
			}
		})
	}
}

func TestBuildCandidateID(t *testing.T) {
	testCases := []struct {
		name      string
		candidate mediagappkg.SearchCandidate
		want      string
	}{
		{
			name: "explicit id wins",
			candidate: mediagappkg.SearchCandidate{
				ID:      " candidate-1 ",
				Title:   "Title",
				Payload: map[string]interface{}{"id": "payload-id"},
			},
			want: "candidate-1",
		},
		{
			name: "payload id fallback",
			candidate: mediagappkg.SearchCandidate{
				Title:   "Title",
				Payload: map[string]interface{}{"guid": " guid-1 ", "hash": "hash-1"},
			},
			want: "guid-1",
		},
		{
			name: "stringifies payload value",
			candidate: mediagappkg.SearchCandidate{
				Title:   "Title",
				Payload: map[string]interface{}{"hash": 12345},
			},
			want: "12345",
		},
		{
			name: "title fallback",
			candidate: mediagappkg.SearchCandidate{
				Title: " Candidate Title ",
			},
			want: "Candidate Title",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildCandidateID(tc.candidate); got != tc.want {
				t.Fatalf("expected candidate id %q, got %q", tc.want, got)
			}
		})
	}
}

func TestResolveCandidateMatchMode(t *testing.T) {
	testCases := []struct {
		name      string
		candidate mediagappkg.SearchCandidate
		want      string
	}{
		{
			name:      "explicit mode wins",
			candidate: mediagappkg.SearchCandidate{MatchMode: " exact "},
			want:      "exact",
		},
		{
			name:      "pack defaults to season",
			candidate: mediagappkg.SearchCandidate{IsPack: true},
			want:      "season",
		},
		{
			name:      "single episode default",
			candidate: mediagappkg.SearchCandidate{},
			want:      "episode",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveCandidateMatchMode(tc.candidate); got != tc.want {
				t.Fatalf("expected match mode %q, got %q", tc.want, got)
			}
		})
	}
}

func TestExtractMapString(t *testing.T) {
	data := map[string]interface{}{
		"empty":  "   ",
		"nil":    nil,
		"title":  " Candidate Title ",
		"season": 2,
		"pack":   true,
	}

	if got := extractMapString(data, "missing", "nil", "empty", "title"); got != "Candidate Title" {
		t.Fatalf("expected first non-empty string, got %q", got)
	}
	if got := extractMapString(data, "season"); got != "2" {
		t.Fatalf("expected numeric value to be rendered, got %q", got)
	}
	if got := extractMapString(data, "pack"); got != "true" {
		t.Fatalf("expected bool value to be rendered, got %q", got)
	}
	if got := extractMapString(nil, "title"); got != "" {
		t.Fatalf("expected nil map to return blank, got %q", got)
	}
}

func TestWriteMediaGapError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testCases := []struct {
		name       string
		err        error
		statusCode int
		wantError  string
	}{
		{
			name:       "not found",
			err:        mediagappkg.ErrMediaGapNotFound,
			statusCode: http.StatusNotFound,
			wantError:  mediagappkg.ErrMediaGapNotFound.Error(),
		},
		{
			name:       "invalid status",
			err:        mediagappkg.ErrMediaGapInvalidStatus,
			statusCode: http.StatusBadRequest,
			wantError:  mediagappkg.ErrMediaGapInvalidStatus.Error(),
		},
		{
			name:       "search state",
			err:        mediagappkg.ErrMediaGapSearchState,
			statusCode: http.StatusBadRequest,
			wantError:  mediagappkg.ErrMediaGapSearchState.Error(),
		},
		{
			name:       "dispatch state",
			err:        mediagappkg.ErrMediaGapDispatchState,
			statusCode: http.StatusBadRequest,
			wantError:  mediagappkg.ErrMediaGapDispatchState.Error(),
		},
		{
			name:       "candidate",
			err:        mediagappkg.ErrMediaGapCandidate,
			statusCode: http.StatusBadRequest,
			wantError:  mediagappkg.ErrMediaGapCandidate.Error(),
		},
		{
			name:       "not configured",
			err:        mediagappkg.ErrMediaGapNotConfigured,
			statusCode: http.StatusServiceUnavailable,
			wantError:  mediagappkg.ErrMediaGapNotConfigured.Error(),
		},
		{
			name:       "wrapped domain error",
			err:        errors.Join(errors.New("wrap"), mediagappkg.ErrMediaGapCandidate),
			statusCode: http.StatusBadRequest,
			wantError:  "wrap\n候选资源不能为空",
		},
		{
			name:       "internal",
			err:        errors.New("db failed"),
			statusCode: http.StatusInternalServerError,
			wantError:  "上游服务暂不可用",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/media-gaps/gap-1/dispatch", nil)

			writeMediaGapError(ctx, tc.err)

			if recorder.Code != tc.statusCode {
				t.Fatalf("expected status %d, got %d", tc.statusCode, recorder.Code)
			}

			var resp struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if resp.Error != tc.wantError {
				t.Fatalf("expected error %q, got %q", tc.wantError, resp.Error)
			}
		})
	}
}
