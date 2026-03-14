package handlers

import "testing"

func TestResolveEmbyWebhookTokenUsesSingleEnvKey(t *testing.T) {
	t.Setenv("EMBY_WEBHOOK_TOKEN", "emby-token")
	t.Setenv("WEBHOOK_TOKEN", "legacy-token")

	if got := resolveEmbyWebhookToken(); got != "emby-token" {
		t.Fatalf("expected EMBY_WEBHOOK_TOKEN, got %q", got)
	}
}
