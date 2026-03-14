package media

import (
	"testing"

	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
)

func TestBuildQualityReportSkipItemWithoutVideoStream(t *testing.T) {
	items := []embyint.EmbyLibraryItem{
		{
			ID:           "item_1",
			Name:         "No Stream",
			MediaStreams: nil,
			MediaSources: nil,
		},
	}

	report := buildQualityReport(items)
	if report == nil {
		t.Fatal("report should not be nil")
	}
	if len(report.ResolutionDistribution) != 0 {
		t.Fatalf("unexpected resolution distribution: %+v", report.ResolutionDistribution)
	}
	if len(report.LowQualityItems) != 0 {
		t.Fatalf("unexpected low quality list: %+v", report.LowQualityItems)
	}
}

func TestBuildQualityReportLowQualityFieldsComplete(t *testing.T) {
	items := []embyint.EmbyLibraryItem{
		{
			ID:   "item_low_1",
			Name: "Old Movie",
			MediaStreams: []embyint.EmbyMediaStream{
				{
					Type:    "Video",
					Codec:   "h264",
					Width:   1280,
					Height:  720,
					BitRate: 2200000,
				},
			},
		},
	}

	report := buildQualityReport(items)
	if report == nil {
		t.Fatal("report should not be nil")
	}
	if len(report.LowQualityItems) != 1 {
		t.Fatalf("expected one low quality item, got: %d", len(report.LowQualityItems))
	}

	item := report.LowQualityItems[0]
	if item.ID == "" || item.Name == "" || item.Resolution == "" || item.Codec == "" {
		t.Fatalf("low quality item fields should be complete, got: %+v", item)
	}
	if item.Resolution != "720p" {
		t.Fatalf("unexpected resolution: %s", item.Resolution)
	}
	if item.Bitrate <= 0 {
		t.Fatalf("bitrate should be normalized to positive kbps, got: %d", item.Bitrate)
	}
	if item.ItemType != "Movie" {
		t.Fatalf("unexpected item type: %s", item.ItemType)
	}
	if item.ItemCount != 1 {
		t.Fatalf("unexpected item count: %d", item.ItemCount)
	}
}

func TestBuildQualityReportAggregateEpisodesBySeries(t *testing.T) {
	items := []embyint.EmbyLibraryItem{
		{
			ID:         "ep_1",
			Name:       "Ep1",
			Type:       "Episode",
			SeriesID:   "series_1",
			SeriesName: "Great Show",
			MediaStreams: []embyint.EmbyMediaStream{
				{Type: "Video", Codec: "h264", Width: 1280, Height: 720, BitRate: 1800000},
			},
		},
		{
			ID:         "ep_2",
			Name:       "Ep2",
			Type:       "Episode",
			SeriesID:   "series_1",
			SeriesName: "Great Show",
			MediaStreams: []embyint.EmbyMediaStream{
				{Type: "Video", Codec: "h264", Width: 1280, Height: 720, BitRate: 2000000},
			},
		},
	}

	report := buildQualityReport(items)
	if report == nil {
		t.Fatal("report should not be nil")
	}
	if len(report.LowQualityItems) != 1 {
		t.Fatalf("expected one grouped item, got: %d", len(report.LowQualityItems))
	}
	group := report.LowQualityItems[0]
	if group.ItemType != "Series" {
		t.Fatalf("unexpected item type: %s", group.ItemType)
	}
	if group.Name != "Great Show" {
		t.Fatalf("unexpected group name: %s", group.Name)
	}
	if group.ItemCount != 2 {
		t.Fatalf("unexpected group count: %d", group.ItemCount)
	}
}

func TestBuildQualityReportAggregateEpisodesBySeriesNameWhenSeriesIDMissing(t *testing.T) {
	items := []embyint.EmbyLibraryItem{
		{
			ID:         "ep_1",
			Name:       "S1E1",
			Type:       "Episode",
			ParentID:   "season_1",
			SeriesName: "No ID Show",
			MediaStreams: []embyint.EmbyMediaStream{
				{Type: "Video", Codec: "h264", Width: 1280, Height: 720, BitRate: 1800000},
			},
		},
		{
			ID:         "ep_2",
			Name:       "S2E1",
			Type:       "Episode",
			ParentID:   "season_2",
			SeriesName: "No ID Show",
			MediaStreams: []embyint.EmbyMediaStream{
				{Type: "Video", Codec: "h264", Width: 1280, Height: 720, BitRate: 1900000},
			},
		},
	}

	report := buildQualityReport(items)
	if report == nil {
		t.Fatal("report should not be nil")
	}
	if len(report.LowQualityItems) != 1 {
		t.Fatalf("expected one grouped item, got: %d", len(report.LowQualityItems))
	}
	group := report.LowQualityItems[0]
	if group.ItemType != "Series" {
		t.Fatalf("unexpected item type: %s", group.ItemType)
	}
	if group.ItemCount != 2 {
		t.Fatalf("unexpected group count: %d", group.ItemCount)
	}
	if group.GroupID == "" {
		t.Fatal("groupId should not be empty")
	}
}

func TestMatchesGroupIDCompatibleWithLegacyRawID(t *testing.T) {
	if !matchesGroupID("series:abc", "series:abc") {
		t.Fatal("same groupId should match")
	}
	if !matchesGroupID("abc", "series:abc") {
		t.Fatal("legacy raw id should match prefixed groupId")
	}
	if !matchesGroupID("series:abc", "abc") {
		t.Fatal("prefixed id should match legacy raw groupId")
	}
	if matchesGroupID("series:abc", "series:def") {
		t.Fatal("different groupId should not match")
	}
}

func TestShouldRefreshLegacyMediaQualityCache(t *testing.T) {
	legacy := &QualityReport{
		LowQualityItems: []LowQualityItem{
			{ID: "series_1", ItemType: "Series"},
		},
	}
	if !shouldRefreshLegacyMediaQualityCache(legacy) {
		t.Fatal("legacy cache without groupId should be refreshed")
	}

	latest := &QualityReport{
		LowQualityItems: []LowQualityItem{
			{ID: "series_1", GroupID: "series:series_1", ItemType: "Series"},
		},
	}
	if shouldRefreshLegacyMediaQualityCache(latest) {
		t.Fatal("latest cache with groupId should not be refreshed")
	}
}

func TestIsAllLibrariesIDCaseInsensitive(t *testing.T) {
	if !isAllLibrariesID("all") {
		t.Fatal("all should be recognized")
	}
	if !isAllLibrariesID("ALL") {
		t.Fatal("ALL should be recognized")
	}
	if !isAllLibrariesID(" All ") {
		t.Fatal("trimmed value should be recognized")
	}
	if isAllLibrariesID("library_1") {
		t.Fatal("regular library id should not be treated as all")
	}
}
