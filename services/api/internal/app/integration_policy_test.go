package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/konghang/ember/backend/internal/models"
)

type integrationFakeEmbyServer struct {
	server         *httptest.Server
	userPolicy     map[string]any
	lastPolicyBody map[string]any
}

func newIntegrationFakeEmbyServer(t *testing.T) *integrationFakeEmbyServer {
	t.Helper()

	fake := &integrationFakeEmbyServer{
		userPolicy: map[string]any{
			"IsAdministrator":                false,
			"IsDisabled":                     false,
			"EnableContentDeletion":          false,
			"EnableContentDownloading":       false,
			"EnableAllFolders":               false,
			"EnabledFolders":                 []any{},
			"EnableLiveTvAccess":             false,
			"EnableSyncTranscoding":          false,
			"EnableMediaPlayback":            true,
			"EnableAudioPlaybackTranscoding": false,
			"EnableVideoPlaybackTranscoding": false,
			"EnablePlaybackRemuxing":         true,
			"EnableRemoteAccess":             true,
			"SimultaneousStreamLimit":        3,
		},
	}

	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/emby/Library/VirtualFolders/Query":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `[
				{"Id":"/data/movies","Name":"电影","CollectionType":"movies","ItemCount":12},
				{"Id":"/data/series","Name":"剧集","CollectionType":"tvshows","ItemCount":8}
			]`)
			return
		case r.Method == http.MethodGet && r.URL.Path == "/emby/Users":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `[
				{"Id":"emby_admin","Name":"integration-admin","Policy":{"IsAdministrator":true}},
				{"Id":"emby_user_policy","Name":"integration-user","Policy":{"IsAdministrator":false}}
			]`)
			return
		case r.Method == http.MethodGet && r.URL.Path == "/emby/Users/emby_admin/Views":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"Items":[
				{"Id":"/data/movies","Name":"电影","CollectionType":"movies"},
				{"Id":"/data/series","Name":"剧集","CollectionType":"tvshows"}
			]}`)
			return
		case r.Method == http.MethodGet && r.URL.Path == "/emby/Users/emby_user_policy":
			w.Header().Set("Content-Type", "application/json")
			body, _ := json.Marshal(map[string]any{
				"Id":     "emby_user_policy",
				"Name":   "integration-user",
				"Policy": fake.userPolicy,
			})
			_, _ = w.Write(body)
			return
		case r.Method == http.MethodPost && r.URL.Path == "/emby/Users/emby_user_policy/Policy":
			defer r.Body.Close()
			if err := json.NewDecoder(r.Body).Decode(&fake.lastPolicyBody); err != nil {
				t.Fatalf("decode posted policy: %v", err)
			}
			fake.userPolicy = fake.lastPolicyBody
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			t.Fatalf("unexpected fake emby request: %s %s", r.Method, r.URL.String())
		}
	}))
	t.Cleanup(fake.server.Close)

	return fake
}

func TestIntegrationUpdatePlanGroupMediaLibrariesDeferred(t *testing.T) {
	harness := newIntegrationHarness(t)
	fakeEmby := newIntegrationFakeEmbyServer(t)
	harness.setSetting(t, "EMBY_URL", fakeEmby.server.URL)
	harness.setSetting(t, "EMBY_API_KEY", "integration-emby-key")

	harness.seedPlanGroup(t, models.PlanGroup{
		Key:                         "VIP",
		Name:                        "VIP",
		MediaLibraryTemplateVersion: 1,
	})
	harness.seedUser(t, models.User{
		Username:  "itest_policy_user",
		Email:     "itest-policy-user@example.com",
		EmbyID:    "emby_policy_user",
		PlanGroup: stringPtr("VIP"),
	})

	recorder := harness.performAdminRequest(http.MethodPut, "/api/v1/admin/plan-groups/VIP/media-libraries", []byte(`{
		"libraryIds":["/data/movies"],
		"applyToExistingUsers":false
	}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Data struct {
			Mode               string `json:"mode"`
			Status             string `json:"status"`
			AffectedUserCount  int    `json:"affectedUserCount"`
			OutOfSyncUserCount int    `json:"outOfSyncUserCount"`
			BatchID            string `json:"batchId"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.Mode != "deferred" || resp.Data.Status != "out_of_sync" || resp.Data.AffectedUserCount != 1 || resp.Data.OutOfSyncUserCount != 1 || resp.Data.BatchID != "" {
		t.Fatalf("unexpected response: %+v", resp.Data)
	}

	var group models.PlanGroup
	if err := harness.database.Where("key = ?", "VIP").First(&group).Error; err != nil {
		t.Fatalf("load plan group: %v", err)
	}
	if group.MediaLibraryTemplateVersion != 2 {
		t.Fatalf("expected template version 2, got %d", group.MediaLibraryTemplateVersion)
	}

	var libraries []models.PlanGroupMediaLibrary
	if err := harness.database.Where("plan_group_key = ?", "VIP").Find(&libraries).Error; err != nil {
		t.Fatalf("load plan group libraries: %v", err)
	}
	if len(libraries) != 1 || libraries[0].LibraryID != "/data/movies" || libraries[0].LibraryName != "电影" {
		t.Fatalf("unexpected persisted libraries: %+v", libraries)
	}

	var batchCount int64
	if err := harness.database.Model(&models.EmbyPolicySyncBatch{}).Where("plan_group_key = ?", "VIP").Count(&batchCount).Error; err != nil {
		t.Fatalf("count sync batches: %v", err)
	}
	if batchCount != 0 {
		t.Fatalf("expected no sync batch for deferred save, got %d", batchCount)
	}
}

func TestIntegrationUserMediaLibraryPolicyApplyCurrentSyncsTemplateVersion(t *testing.T) {
	harness := newIntegrationHarness(t)
	fakeEmby := newIntegrationFakeEmbyServer(t)
	harness.setSetting(t, "EMBY_URL", fakeEmby.server.URL)
	harness.setSetting(t, "EMBY_API_KEY", "integration-emby-key")

	harness.seedPlanGroup(t, models.PlanGroup{
		Key:                         "VIP",
		Name:                        "VIP",
		MediaLibraryTemplateVersion: 3,
	})
	harness.seedPlanGroupTemplate(t, models.PlanGroupEmbyPolicyTemplate{
		PlanGroupKey:            "VIP",
		SimultaneousStreamLimit: 3,
		EnablePlaybackRemuxing:  true,
		EnableRemoteAccess:      true,
	})
	harness.seedPlanGroupLibraries(t, models.PlanGroupMediaLibrary{
		PlanGroupKey: "VIP",
		LibraryID:    "/data/movies",
		LibraryName:  "电影",
		LibraryType:  "movies",
		SortOrder:    0,
	})

	user := harness.seedUser(t, models.User{
		Username:                           "itest_policy_apply",
		Email:                              "itest-policy-apply@example.com",
		EmbyID:                             "emby_user_policy",
		PlanGroup:                          stringPtr("VIP"),
		AppliedMediaLibraryTemplateVersion: 1,
	})

	before := harness.performUserRequest(t, user, http.MethodGet, "/api/v1/user/media-libraries", nil)
	if before.Code != http.StatusOK {
		t.Fatalf("expected 200 before sync, got %d body=%s", before.Code, before.Body.String())
	}
	var beforeResp struct {
		Data struct {
			PolicySyncStatus string `json:"policySyncStatus"`
		} `json:"data"`
	}
	if err := json.Unmarshal(before.Body.Bytes(), &beforeResp); err != nil {
		t.Fatalf("decode before response: %v", err)
	}
	if beforeResp.Data.PolicySyncStatus != "out_of_sync" {
		t.Fatalf("expected out_of_sync before apply, got %+v", beforeResp.Data)
	}

	recorder := harness.performUserRequest(t, user, http.MethodPost, "/api/v1/user/emby-policy-sync/apply-current", []byte(`{}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Data struct {
			UserID           string `json:"userId"`
			PlanGroup        string `json:"planGroup"`
			TemplateCount    int    `json:"templateCount"`
			EnabledCount     int    `json:"enabledCount"`
			PolicySyncStatus string `json:"policySyncStatus"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.UserID != user.ID || resp.Data.PlanGroup != "VIP" || resp.Data.TemplateCount != 1 || resp.Data.EnabledCount != 1 || resp.Data.PolicySyncStatus != "synced" {
		t.Fatalf("unexpected response: %+v", resp.Data)
	}

	var refreshed models.User
	if err := harness.database.Where("id = ?", user.ID).First(&refreshed).Error; err != nil {
		t.Fatalf("load refreshed user: %v", err)
	}
	if refreshed.AppliedMediaLibraryTemplateVersion != 3 {
		t.Fatalf("expected applied template version 3, got %d", refreshed.AppliedMediaLibraryTemplateVersion)
	}
	if refreshed.EmbyDisabled {
		t.Fatal("expected embyDisabled to remain false")
	}

	if fakeEmby.lastPolicyBody == nil {
		t.Fatal("expected fake emby to receive patched policy")
	}
	enabledFolders, ok := fakeEmby.lastPolicyBody["EnabledFolders"].([]any)
	if !ok || len(enabledFolders) != 1 || strings.TrimSpace(enabledFolders[0].(string)) != "/data/movies" {
		t.Fatalf("expected EnabledFolders to include /data/movies, got %+v", fakeEmby.lastPolicyBody["EnabledFolders"])
	}
}

func TestIntegrationRankingLibraryAllowlistPersistsAndReloads(t *testing.T) {
	harness := newIntegrationHarness(t)
	fakeEmby := newIntegrationFakeEmbyServer(t)
	harness.setSetting(t, "EMBY_URL", fakeEmby.server.URL)
	harness.setSetting(t, "EMBY_API_KEY", "integration-emby-key")

	initial := harness.performAdminRequest(http.MethodGet, "/api/v1/admin/rankings/library-allowlist", nil)
	if initial.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", initial.Code, initial.Body.String())
	}
	var initialResp struct {
		Data struct {
			AllowAll   bool     `json:"allowAll"`
			LibraryIDs []string `json:"libraryIds"`
		} `json:"data"`
	}
	if err := json.Unmarshal(initial.Body.Bytes(), &initialResp); err != nil {
		t.Fatalf("decode initial response: %v", err)
	}
	if !initialResp.Data.AllowAll || len(initialResp.Data.LibraryIDs) != 0 {
		t.Fatalf("expected initial allow-all response, got %+v", initialResp.Data)
	}

	update := harness.performAdminRequest(http.MethodPut, "/api/v1/admin/rankings/library-allowlist", []byte(`{
		"libraryIds":["/data/movies"]
	}`))
	if update.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", update.Code, update.Body.String())
	}
	var updatedResp struct {
		Data struct {
			AllowAll   bool     `json:"allowAll"`
			LibraryIDs []string `json:"libraryIds"`
		} `json:"data"`
	}
	if err := json.Unmarshal(update.Body.Bytes(), &updatedResp); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if updatedResp.Data.AllowAll || len(updatedResp.Data.LibraryIDs) != 1 || updatedResp.Data.LibraryIDs[0] != "/data/movies" {
		t.Fatalf("unexpected updated allowlist: %+v", updatedResp.Data)
	}

	var setting models.Setting
	if err := harness.database.Where("key = ?", "playback_ranking_library_allowlist").First(&setting).Error; err != nil {
		t.Fatalf("load persisted allowlist setting: %v", err)
	}
	if setting.Value != `["/data/movies"]` {
		t.Fatalf("unexpected persisted allowlist value: %s", setting.Value)
	}

	reloaded := harness.performAdminRequest(http.MethodGet, "/api/v1/admin/rankings/library-allowlist", nil)
	if reloaded.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", reloaded.Code, reloaded.Body.String())
	}
	var reloadedResp struct {
		Data struct {
			AllowAll   bool     `json:"allowAll"`
			LibraryIDs []string `json:"libraryIds"`
		} `json:"data"`
	}
	if err := json.Unmarshal(reloaded.Body.Bytes(), &reloadedResp); err != nil {
		t.Fatalf("decode reloaded response: %v", err)
	}
	if reloadedResp.Data.AllowAll || len(reloadedResp.Data.LibraryIDs) != 1 || reloadedResp.Data.LibraryIDs[0] != "/data/movies" {
		t.Fatalf("unexpected reloaded allowlist: %+v", reloadedResp.Data)
	}
}

func stringPtr(value string) *string {
	return &value
}
