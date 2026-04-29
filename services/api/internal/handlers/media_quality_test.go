package handlers

import (
	"testing"
	"time"

	mediapkg "github.com/konghang/ember/backend/internal/services/media"
)

func TestPaginateMediaQualityReportKeepsFailedLibraries(t *testing.T) {
	report := &mediapkg.QualityReport{
		LowQualityItems: []mediapkg.LowQualityItem{
			{ID: "item_1", Name: "A"},
			{ID: "item_2", Name: "B"},
		},
		LowQualityDetails: []mediapkg.LowQualityDetailItem{
			{ID: "detail_1"},
		},
		FailedLibraries: []mediapkg.FailedLibrary{
			{LibraryID: "lib_1", LibraryName: "Movies", Error: "emby timeout"},
		},
		ScanAt: time.Now().UTC(),
	}

	paginated := paginateMediaQualityReport(report, 1, 1)
	if paginated == nil {
		t.Fatal("expected paginated report")
	}
	if len(paginated.LowQualityItems) != 1 {
		t.Fatalf("expected 1 paginated item, got %d", len(paginated.LowQualityItems))
	}
	if paginated.LowQualityItems[0].ID != "item_1" {
		t.Fatalf("unexpected paginated item: %+v", paginated.LowQualityItems[0])
	}
	if len(paginated.LowQualityDetails) != 0 {
		t.Fatalf("expected details to be stripped from paginated report, got %+v", paginated.LowQualityDetails)
	}
	if len(paginated.FailedLibraries) != 1 {
		t.Fatalf("expected failedLibraries to be preserved, got %+v", paginated.FailedLibraries)
	}
	if paginated.FailedLibraries[0].LibraryID != "lib_1" {
		t.Fatalf("unexpected failed library: %+v", paginated.FailedLibraries[0])
	}
}
