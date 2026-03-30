package handlers

import (
	"reflect"
	"testing"
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
