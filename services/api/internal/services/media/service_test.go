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
		{ID: "", Name: "Dark", Type: "Series", ProductionYear: 2017},
		{ID: "", Name: "Dark", Type: "Series", ProductionYear: 2017},
		{ID: "", Name: "Dark", Type: "Series", ProductionYear: 2020},
	}

	got := dedupeLatestItems(items)
	if len(got) != 2 {
		t.Fatalf("expected 2 unique items, got %d", len(got))
	}
}
