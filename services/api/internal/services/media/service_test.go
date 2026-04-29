package media

import (
	"testing"

	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
)

func TestDedupeLatestItemsRemovesDuplicateIDs(t *testing.T) {
	items := []embyint.EmbyItem{
		{ID: "movie_1", Name: "Inception", Type: "Movie", ProductionYear: 2010},
		{ID: "movie_1", Name: "Inception", Type: "Movie", ProductionYear: 2010},
		{ID: "movie_2", Name: "Interstellar", Type: "Movie", ProductionYear: 2014},
	}

	got := dedupeLatestItems(items)
	if len(got) != 2 {
		t.Fatalf("expected 2 unique items, got %d", len(got))
	}
	if got[0].ID != "movie_1" || got[1].ID != "movie_2" {
		t.Fatalf("unexpected item order after dedupe: %+v", got)
	}
}

func TestDedupeLatestItemsFallsBackToTypeNameYear(t *testing.T) {
	items := []embyint.EmbyItem{
		{ID: "", Name: "Dark", Type: "Series", ProductionYear: 2017, DateCreated: "2026-04-01T00:00:00Z"},
		{ID: "", Name: "Dark", Type: "Series", ProductionYear: 2017, DateCreated: "2026-04-01T00:00:00Z"},
		{ID: "", Name: "Dark", Type: "Series", ProductionYear: 2020, DateCreated: "2026-04-02T00:00:00Z"},
	}

	got := dedupeLatestItems(items)
	if len(got) != 2 {
		t.Fatalf("expected 2 unique items, got %d", len(got))
	}
}

func TestDedupeLatestItemsKeepsDistinctFallbackItems(t *testing.T) {
	items := []embyint.EmbyItem{
		{
			Name:           "Dark",
			Type:           "Series",
			ProductionYear: 2017,
			DateCreated:    "2026-04-01T00:00:00Z",
			ImageTags:      map[string]string{"Primary": "poster-a"},
		},
		{
			Name:           "Dark",
			Type:           "Series",
			ProductionYear: 2017,
			DateCreated:    "2026-04-03T00:00:00Z",
			ImageTags:      map[string]string{"Primary": "poster-b"},
		},
	}

	got := dedupeLatestItems(items)
	if len(got) != 2 {
		t.Fatalf("expected distinct fallback items to be kept, got %d", len(got))
	}
}
