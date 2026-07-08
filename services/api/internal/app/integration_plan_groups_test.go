package app

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestIntegrationAdminGetPlanGroups(t *testing.T) {
	harness := newIntegrationHarness(t)

	recorder := harness.performAdminRequest(http.MethodGet, "/api/v1/admin/plan-groups", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Data []struct {
			Key       string `json:"key"`
			Name      string `json:"name"`
			IsDefault bool   `json:"isDefault"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data) == 0 {
		t.Fatalf("expected at least one plan group, got empty response")
	}

	foundDefault := false
	for _, item := range resp.Data {
		if item.Key == "DEFAULT" && item.IsDefault {
			foundDefault = true
			break
		}
	}
	if !foundDefault {
		t.Fatalf("expected DEFAULT plan group in response, got %+v", resp.Data)
	}
}
