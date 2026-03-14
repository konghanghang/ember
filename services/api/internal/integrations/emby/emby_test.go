package emby

import "testing"

func TestLatestIncludeItemTypes(t *testing.T) {
	if got := latestIncludeItemTypes("Movie"); got != "Movie" {
		t.Fatalf("expected Movie to stay Movie, got %s", got)
	}
	if got := latestIncludeItemTypes("Series"); got != "Episode" {
		t.Fatalf("expected Series latest query to use Episode, got %s", got)
	}
}
