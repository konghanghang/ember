package handlers

import (
	"encoding/json"
	"testing"
)

func TestResolveEmbyWebhookTokenUsesSingleEnvKey(t *testing.T) {
	t.Setenv("EMBY_WEBHOOK_TOKEN", "emby-token")
	t.Setenv("WEBHOOK_TOKEN", "legacy-token")

	if got := resolveEmbyWebhookToken(); got != "emby-token" {
		t.Fatalf("expected EMBY_WEBHOOK_TOKEN, got %q", got)
	}
}

func TestExtractSeriesID(t *testing.T) {
	tests := []struct {
		name string
		item map[string]interface{}
		want string
	}{
		{
			name: "uses explicit series id",
			item: map[string]interface{}{
				"SeriesId": "series_123",
				"ParentId": "season_456",
			},
			want: "series_123",
		},
		{
			name: "does not fallback to parent id",
			item: map[string]interface{}{
				"ParentId": "season_456",
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractSeriesID(tt.item); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestTVCalendarWebhookExtractStringAndMap(t *testing.T) {
	payload := map[string]interface{}{
		"name": "  Episode One  ",
		"Item": map[string]interface{}{
			"Id": "item_1",
		},
		"invalid": "not a map",
	}

	if got := extractString(payload, "Name", "name"); got != "Episode One" {
		t.Fatalf("expected trimmed fallback string, got %q", got)
	}
	if got := extractString(payload, "missing"); got != "" {
		t.Fatalf("expected missing string to be empty, got %q", got)
	}

	item := extractMap(payload, "item", "Item")
	if item == nil || item["Id"] != "item_1" {
		t.Fatalf("expected Item map, got %+v", item)
	}
	if got := extractMap(payload, "invalid"); got != nil {
		t.Fatalf("expected non-map value to be ignored, got %+v", got)
	}
}

func TestTVCalendarWebhookExtractInt(t *testing.T) {
	tests := []struct {
		name string
		data map[string]interface{}
		keys []string
		want int
	}{
		{
			name: "float64 from json decode",
			data: map[string]interface{}{"seasonNumber": float64(3)},
			keys: []string{"SeasonNumber", "seasonNumber"},
			want: 3,
		},
		{
			name: "int64 value",
			data: map[string]interface{}{"EpisodeNumber": int64(12)},
			keys: []string{"EpisodeNumber"},
			want: 12,
		},
		{
			name: "json number",
			data: map[string]interface{}{"IndexNumber": json.Number("8")},
			keys: []string{"IndexNumber"},
			want: 8,
		},
		{
			name: "trimmed string",
			data: map[string]interface{}{"episodeNumber": " 9 "},
			keys: []string{"EpisodeNumber", "episodeNumber"},
			want: 9,
		},
		{
			name: "invalid values become zero",
			data: map[string]interface{}{"EpisodeNumber": "x"},
			keys: []string{"EpisodeNumber"},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractInt(tt.data, tt.keys...); got != tt.want {
				t.Fatalf("expected %d, got %d", tt.want, got)
			}
		})
	}
}

func TestTVCalendarWebhookExtractBool(t *testing.T) {
	tests := []struct {
		name string
		data map[string]interface{}
		want bool
	}{
		{name: "bool true", data: map[string]interface{}{"IsMissing": true}, want: true},
		{name: "number true", data: map[string]interface{}{"IsMissing": float64(1)}, want: true},
		{name: "string true", data: map[string]interface{}{"isMissing": " yes "}, want: true},
		{name: "string false", data: map[string]interface{}{"isMissing": "no"}, want: false},
		{name: "missing false", data: map[string]interface{}{}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractBool(tt.data, "IsMissing", "isMissing"); got != tt.want {
				t.Fatalf("expected %t, got %t", tt.want, got)
			}
		})
	}
}

func TestExtractTMDBID(t *testing.T) {
	if got := extractTMDBID(map[string]interface{}{"Provider_tmdb": " 1399 "}); got != "1399" {
		t.Fatalf("expected direct tmdb provider id, got %q", got)
	}

	got := extractTMDBID(map[string]interface{}{
		"ProviderIds": map[string]interface{}{
			"Tmdb": " 456 ",
		},
	})
	if got != "456" {
		t.Fatalf("expected nested tmdb provider id, got %q", got)
	}

	if got := extractTMDBID(map[string]interface{}{"ProviderIds": "invalid"}); got != "" {
		t.Fatalf("expected invalid provider ids to return empty string, got %q", got)
	}
}

func TestHasPhysicalMedia(t *testing.T) {
	tests := []struct {
		name string
		item map[string]interface{}
		want bool
	}{
		{
			name: "path is physical",
			item: map[string]interface{}{"Path": " /media/show/s01e01.mkv "},
			want: true,
		},
		{
			name: "media sources are physical",
			item: map[string]interface{}{"MediaSources": []interface{}{map[string]interface{}{"Id": "source_1"}}},
			want: true,
		},
		{
			name: "missing item is ignored",
			item: map[string]interface{}{"IsMissing": true, "Path": "/media/show/s01e01.mkv"},
			want: false,
		},
		{
			name: "virtual item is ignored",
			item: map[string]interface{}{"LocationType": " virtual ", "Path": "/media/show/s01e01.mkv"},
			want: false,
		},
		{
			name: "empty media sources are ignored",
			item: map[string]interface{}{"MediaSources": []interface{}{}},
			want: false,
		},
		{
			name: "non-slice media sources are ignored",
			item: map[string]interface{}{"MediaSources": "source"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasPhysicalMedia(tt.item); got != tt.want {
				t.Fatalf("expected %t, got %t", tt.want, got)
			}
		})
	}
}
