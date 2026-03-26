package handlers

import "testing"

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
