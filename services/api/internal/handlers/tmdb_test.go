package handlers

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBuildTMDBTVSeasonOptionsUsesExplicitSeasonList(t *testing.T) {
	options := buildTMDBTVSeasonOptions(TMDBTVDetailResponse{
		ID:              1399,
		Name:            "Game of Thrones",
		NumberOfSeasons: 8,
		Seasons: []TMDBTVSeason{
			{SeasonNumber: 0},
			{SeasonNumber: 1},
			{SeasonNumber: 2},
			{SeasonNumber: 2},
			{SeasonNumber: 8},
		},
	})

	expected := []int{1, 2, 8}
	if !reflect.DeepEqual(options.Seasons, expected) {
		t.Fatalf("expected seasons %v, got %v", expected, options.Seasons)
	}
	if options.NumberOfSeasons != 8 {
		t.Fatalf("expected numberOfSeasons to be 8, got %d", options.NumberOfSeasons)
	}
}

func TestBuildTMDBTVSeasonOptionsFallsBackToSeasonCount(t *testing.T) {
	options := buildTMDBTVSeasonOptions(TMDBTVDetailResponse{
		ID:              66732,
		Name:            "Stranger Things",
		NumberOfSeasons: 4,
	})

	expected := []int{1, 2, 3, 4}
	if !reflect.DeepEqual(options.Seasons, expected) {
		t.Fatalf("expected seasons %v, got %v", expected, options.Seasons)
	}
}

func TestTMDBHandlerSearchReturnsGenericInternalErrorWhenAPIKeyMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Setenv("TMDB_API_KEY", "")

	handler := NewTMDBHandler()
	ctx, recorder := newTestConfigContext(http.MethodGet, "/api/v1/tmdb/search?query=inception&type=movie", nil)
	handler.Search(ctx)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", recorder.Code)
	}

	var resp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Error != "上游服务暂不可用" {
		t.Fatalf("expected generic internal error message, got %q", resp.Error)
	}
}
