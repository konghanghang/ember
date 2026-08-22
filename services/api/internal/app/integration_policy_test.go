package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/emby/Users/"):
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			t.Fatalf("unexpected fake emby request: %s %s", r.Method, r.URL.String())
		}
	}))
	t.Cleanup(fake.server.Close)

	return fake
}

func TestIntegrationUpdateUserEmbyAccessRevokesActiveMappings(t *testing.T) {
	harness := newIntegrationHarness(t)
	fakeEmby := newIntegrationFakeEmbyServer(t)
	harness.setSetting(t, "EMBY_URL", fakeEmby.server.URL)
	harness.setSetting(t, "EMBY_API_KEY", "integration-emby-key")

	user := harness.seedUser(t, models.User{
		Username: "itest_emby_access_revoke",
		Email:    "itest-emby-access-revoke@example.com",
		EmbyID:   "emby_user_policy",
	})
	mapping := models.EmbyAccessToken{
		ServerID: "server-1", TokenHash: bytes.Repeat([]byte{0x31}, 32),
		EmbyUserID: user.EmbyID, UserID: &user.ID, DeviceID: "device-1",
		ClientName: "Infuse", LastSeenAt: time.Now().UTC(),
	}
	if err := harness.database.Create(&mapping).Error; err != nil {
		t.Fatalf("create token mapping: %v", err)
	}

	recorder := harness.performAdminRequest(http.MethodPut, "/api/v1/admin/users/"+user.ID+"/emby-access", []byte(`{"disabled":true}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var refreshedUser models.User
	if err := harness.database.Where("id = ?", user.ID).First(&refreshedUser).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if !refreshedUser.EmbyAccessDisabled {
		t.Fatal("emby_access_disabled was not persisted")
	}
	var refreshedMapping models.EmbyAccessToken
	if err := harness.database.Where("id = ?", mapping.ID).First(&refreshedMapping).Error; err != nil {
		t.Fatalf("reload token mapping: %v", err)
	}
	if refreshedMapping.RevokedAt == nil || refreshedMapping.RevokedReason == nil ||
		*refreshedMapping.RevokedReason != "emby_access_disabled" || refreshedMapping.RevokedBy == nil ||
		*refreshedMapping.RevokedBy != harness.adminUser.ID {
		t.Fatalf("unexpected revocation audit: %+v", refreshedMapping)
	}
}

func TestIntegrationAdminExpiryPolicyRevokesBeforeEmbyDisable(t *testing.T) {
	harness := newIntegrationHarness(t)
	fakeEmby := newIntegrationFakeEmbyServer(t)
	harness.setSetting(t, "EMBY_URL", fakeEmby.server.URL)
	harness.setSetting(t, "EMBY_API_KEY", "integration-emby-key")
	user := harness.seedUser(t, models.User{
		Username: "itest_expiry_policy_revoke",
		Email:    "itest-expiry-policy-revoke@example.com",
		EmbyID:   "emby_user_policy",
	})
	mapping := seedIntegrationAccessMapping(t, harness, user, "expiry_policy_mapping", 0x46)
	recorder := harness.performAdminRequest(http.MethodPut, "/api/v1/admin/users/"+user.ID, []byte(`{"expiresAt":"2020-01-01T00:00:00Z"}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	assertIntegrationMappingRevoked(t, harness, mapping.ID, "emby_disabled", "system:policy")
	var refreshed models.User
	if err := harness.database.Where("id = ?", user.ID).First(&refreshed).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if !refreshed.EmbyDisabled {
		t.Fatal("emby_disabled was not persisted")
	}
	legacyRestoreMapping := seedIntegrationAccessMapping(t, harness, user, "expiry_restore_mapping", 0x47)
	restore := harness.performAdminRequest(http.MethodPut, "/api/v1/admin/users/"+user.ID, []byte(`{"expiresAt":"2030-01-01T00:00:00Z"}`))
	if restore.Code != http.StatusOK {
		t.Fatalf("restore expected 200, got %d body=%s", restore.Code, restore.Body.String())
	}
	assertIntegrationMappingRevoked(t, harness, legacyRestoreMapping.ID, "security_revoke", "system:policy")
	if err := harness.database.Where("id = ?", user.ID).First(&refreshed).Error; err != nil {
		t.Fatalf("reload restored user: %v", err)
	}
	if refreshed.EmbyDisabled {
		t.Fatal("emby_disabled was not cleared after safe restore")
	}
}

func TestIntegrationHardStateHandlersRevokeMappingsWithAdminActor(t *testing.T) {
	harness := newIntegrationHarness(t)
	fakeEmby := newIntegrationFakeEmbyServer(t)
	harness.setSetting(t, "EMBY_URL", fakeEmby.server.URL)
	harness.setSetting(t, "EMBY_API_KEY", "integration-emby-key")

	target := harness.seedUser(t, models.User{
		Username: "itest_hard_state_toggle",
		Email:    "itest-hard-state-toggle@example.com",
		EmbyID:   "emby-hard-state-toggle",
	})
	toggleMapping := seedIntegrationAccessMapping(t, harness, target, "toggle_mapping", 0x41)
	toggleResponse := harness.performAdminRequest(http.MethodPut, "/api/v1/admin/users/"+target.ID+"/toggle", nil)
	if toggleResponse.Code != http.StatusOK {
		t.Fatalf("toggle expected 200, got %d body=%s", toggleResponse.Code, toggleResponse.Body.String())
	}
	assertIntegrationMappingRevoked(t, harness, toggleMapping.ID, "user_disabled", harness.adminUser.ID)
	legacyRestoreMapping := seedIntegrationAccessMapping(t, harness, target, "restore_mapping", 0x43)
	restoreResponse := harness.performAdminRequest(http.MethodPut, "/api/v1/admin/users/"+target.ID+"/toggle", nil)
	if restoreResponse.Code != http.StatusOK {
		t.Fatalf("restore expected 200, got %d body=%s", restoreResponse.Code, restoreResponse.Body.String())
	}
	assertIntegrationMappingRevoked(t, harness, legacyRestoreMapping.ID, "security_revoke", harness.adminUser.ID)
	var restoredTarget models.User
	if err := harness.database.Where("id = ?", target.ID).First(&restoredTarget).Error; err != nil {
		t.Fatalf("reload restored user: %v", err)
	}
	if !restoredTarget.IsActive {
		t.Fatal("target user was not restored after stale token revocation")
	}
	adminEditMapping := seedIntegrationAccessMapping(t, harness, target, "admin_edit_mapping", 0x44)
	adminEditResponse := harness.performAdminRequest(http.MethodPut, "/api/v1/admin/users/"+target.ID, []byte(`{"isActive":false}`))
	if adminEditResponse.Code != http.StatusOK {
		t.Fatalf("admin edit expected 200, got %d body=%s", adminEditResponse.Code, adminEditResponse.Body.String())
	}
	assertIntegrationMappingRevoked(t, harness, adminEditMapping.ID, "user_disabled", harness.adminUser.ID)

	adminEmbyID := "emby-admin-binding"
	if err := harness.database.Model(&models.User{}).Where("id = ?", harness.adminUser.ID).Update("emby_id", adminEmbyID).Error; err != nil {
		t.Fatalf("bind integration admin: %v", err)
	}
	boundAdmin := harness.adminUser
	boundAdmin.EmbyID = adminEmbyID
	unbindMapping := seedIntegrationAccessMapping(t, harness, boundAdmin, "unbind_mapping", 0x42)
	unbindResponse := harness.performAdminRequest(http.MethodDelete, "/api/v1/admin/current/emby-binding", nil)
	if unbindResponse.Code != http.StatusOK {
		t.Fatalf("unbind expected 200, got %d body=%s", unbindResponse.Code, unbindResponse.Body.String())
	}
	assertIntegrationMappingRevoked(t, harness, unbindMapping.ID, "emby_unbound", harness.adminUser.ID)
	var refreshedAdmin models.User
	if err := harness.database.Where("id = ?", harness.adminUser.ID).First(&refreshedAdmin).Error; err != nil {
		t.Fatalf("reload admin: %v", err)
	}
	if refreshedAdmin.EmbyID != "" {
		t.Fatalf("admin emby_id = %q, want empty", refreshedAdmin.EmbyID)
	}

	deletedUser := harness.seedUser(t, models.User{
		Username: "itest_hard_state_delete",
		Email:    "itest-hard-state-delete@example.com",
		EmbyID:   "emby-hard-state-delete",
	})
	deleteMapping := seedIntegrationAccessMapping(t, harness, deletedUser, "delete_mapping", 0x45)
	deleteResponse := harness.performAdminRequest(http.MethodDelete, "/api/v1/admin/users/"+deletedUser.ID, nil)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("delete expected 200, got %d body=%s", deleteResponse.Code, deleteResponse.Body.String())
	}
	assertIntegrationMappingRevoked(t, harness, deleteMapping.ID, "user_deleted", harness.adminUser.ID)
	var deletedCount int64
	if err := harness.database.Model(&models.User{}).Where("id = ?", deletedUser.ID).Count(&deletedCount).Error; err != nil {
		t.Fatalf("count deleted user: %v", err)
	}
	if deletedCount != 0 {
		t.Fatalf("deleted user count = %d", deletedCount)
	}
	var deletedMapping models.EmbyAccessToken
	if err := harness.database.Where("id = ?", deleteMapping.ID).First(&deletedMapping).Error; err != nil {
		t.Fatalf("reload deleted mapping: %v", err)
	}
	if deletedMapping.UserID != nil {
		t.Fatalf("deleted mapping userId = %v, want nil", deletedMapping.UserID)
	}
}

func seedIntegrationAccessMapping(t *testing.T, harness *integrationHarness, user models.User, id string, digestByte byte) models.EmbyAccessToken {
	t.Helper()
	mapping := models.EmbyAccessToken{
		ID: id, ServerID: "server-1", TokenHash: bytes.Repeat([]byte{digestByte}, 32),
		EmbyUserID: user.EmbyID, UserID: &user.ID, DeviceID: "device-1",
		ClientName: "Infuse", LastSeenAt: time.Now().UTC(),
	}
	if err := harness.database.Create(&mapping).Error; err != nil {
		t.Fatalf("create token mapping %s: %v", id, err)
	}
	return mapping
}

func assertIntegrationMappingRevoked(t *testing.T, harness *integrationHarness, mappingID, reason, actor string) {
	t.Helper()
	var mapping models.EmbyAccessToken
	if err := harness.database.Where("id = ?", mappingID).First(&mapping).Error; err != nil {
		t.Fatalf("reload mapping %s: %v", mappingID, err)
	}
	if mapping.RevokedAt == nil || mapping.RevokedReason == nil || *mapping.RevokedReason != reason ||
		mapping.RevokedBy == nil || *mapping.RevokedBy != actor {
		t.Fatalf("mapping %s audit = %+v", mappingID, mapping)
	}
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

func TestIntegrationUpdatePlanGroupMediaLibrariesDeferredNoopKeepsVersion(t *testing.T) {
	harness := newIntegrationHarness(t)
	fakeEmby := newIntegrationFakeEmbyServer(t)
	harness.setSetting(t, "EMBY_URL", fakeEmby.server.URL)
	harness.setSetting(t, "EMBY_API_KEY", "integration-emby-key")

	harness.seedPlanGroup(t, models.PlanGroup{
		Key:                         "VIP",
		Name:                        "VIP",
		MediaLibraryTemplateVersion: 2,
	})
	harness.seedPlanGroupLibraries(t, models.PlanGroupMediaLibrary{
		PlanGroupKey: "VIP",
		LibraryID:    "/data/movies",
		LibraryName:  "电影",
		LibraryType:  "movies",
		SortOrder:    0,
	})
	user := harness.seedUser(t, models.User{
		Username:                           "itest_policy_noop_user",
		Email:                              "itest-policy-noop-user@example.com",
		EmbyID:                             "emby_user_policy",
		PlanGroup:                          stringPtr("VIP"),
		AppliedMediaLibraryTemplateVersion: 2,
	})

	before := harness.performUserRequest(t, user, http.MethodGet, "/api/v1/user/media-libraries", nil)
	if before.Code != http.StatusOK {
		t.Fatalf("expected 200 before noop save, got %d body=%s", before.Code, before.Body.String())
	}

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
	if resp.Data.Mode != "deferred" || resp.Data.Status != "synced" || resp.Data.OutOfSyncUserCount != 0 || resp.Data.BatchID != "" {
		t.Fatalf("unexpected noop deferred response: %+v", resp.Data)
	}

	var group models.PlanGroup
	if err := harness.database.Where("key = ?", "VIP").First(&group).Error; err != nil {
		t.Fatalf("load plan group: %v", err)
	}
	if group.MediaLibraryTemplateVersion != 2 {
		t.Fatalf("expected template version to remain 2, got %d", group.MediaLibraryTemplateVersion)
	}

	var batchCount int64
	if err := harness.database.Model(&models.EmbyPolicySyncBatch{}).Where("plan_group_key = ?", "VIP").Count(&batchCount).Error; err != nil {
		t.Fatalf("count sync batches: %v", err)
	}
	if batchCount != 0 {
		t.Fatalf("expected no sync batch for noop deferred save, got %d", batchCount)
	}

	after := harness.performUserRequest(t, user, http.MethodGet, "/api/v1/user/media-libraries", nil)
	if after.Code != http.StatusOK {
		t.Fatalf("expected 200 after noop save, got %d body=%s", after.Code, after.Body.String())
	}
	var afterResp struct {
		Data struct {
			PolicySyncStatus string `json:"policySyncStatus"`
		} `json:"data"`
	}
	if err := json.Unmarshal(after.Body.Bytes(), &afterResp); err != nil {
		t.Fatalf("decode after response: %v", err)
	}
	if afterResp.Data.PolicySyncStatus != "synced" {
		t.Fatalf("expected synced after noop deferred save, got %+v", afterResp.Data)
	}
}

func TestIntegrationUpdatePlanGroupMediaLibrariesBatchNoopSkipsSync(t *testing.T) {
	harness := newIntegrationHarness(t)
	fakeEmby := newIntegrationFakeEmbyServer(t)
	harness.setSetting(t, "EMBY_URL", fakeEmby.server.URL)
	harness.setSetting(t, "EMBY_API_KEY", "integration-emby-key")

	harness.seedPlanGroup(t, models.PlanGroup{
		Key:                         "VIP",
		Name:                        "VIP",
		MediaLibraryTemplateVersion: 2,
	})
	harness.seedPlanGroupLibraries(t, models.PlanGroupMediaLibrary{
		PlanGroupKey: "VIP",
		LibraryID:    "/data/movies",
		LibraryName:  "电影",
		LibraryType:  "movies",
		SortOrder:    0,
	})
	harness.seedUser(t, models.User{
		Username:                           "itest_policy_batch_noop_user",
		Email:                              "itest-policy-batch-noop@example.com",
		EmbyID:                             "emby_user_policy",
		PlanGroup:                          stringPtr("VIP"),
		AppliedMediaLibraryTemplateVersion: 2,
	})

	recorder := harness.performAdminRequest(http.MethodPut, "/api/v1/admin/plan-groups/VIP/media-libraries", []byte(`{
		"libraryIds":["/data/movies"],
		"applyToExistingUsers":true
	}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Data struct {
			Mode              string `json:"mode"`
			Status            string `json:"status"`
			AffectedUserCount int    `json:"affectedUserCount"`
			BatchID           string `json:"batchId"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.Mode != "batch" || resp.Data.Status != "synced" || resp.Data.AffectedUserCount != 0 || resp.Data.BatchID != "" {
		t.Fatalf("unexpected noop batch response: %+v", resp.Data)
	}

	var group models.PlanGroup
	if err := harness.database.Where("key = ?", "VIP").First(&group).Error; err != nil {
		t.Fatalf("load plan group: %v", err)
	}
	if group.MediaLibraryTemplateVersion != 2 {
		t.Fatalf("expected template version to remain 2, got %d", group.MediaLibraryTemplateVersion)
	}

	var batchCount int64
	if err := harness.database.Model(&models.EmbyPolicySyncBatch{}).Where("plan_group_key = ?", "VIP").Count(&batchCount).Error; err != nil {
		t.Fatalf("count sync batches: %v", err)
	}
	if batchCount != 0 {
		t.Fatalf("expected no sync batch for noop batch save, got %d", batchCount)
	}
}

func TestIntegrationApplyPlanGroupMediaLibrarySyncNoopKeepsVersion(t *testing.T) {
	harness := newIntegrationHarness(t)
	fakeEmby := newIntegrationFakeEmbyServer(t)
	harness.setSetting(t, "EMBY_URL", fakeEmby.server.URL)
	harness.setSetting(t, "EMBY_API_KEY", "integration-emby-key")

	harness.seedPlanGroup(t, models.PlanGroup{
		Key:                         "VIP",
		Name:                        "VIP",
		MediaLibraryTemplateVersion: 2,
	})
	harness.seedPlanGroupLibraries(t, models.PlanGroupMediaLibrary{
		PlanGroupKey: "VIP",
		LibraryID:    "/data/movies",
		LibraryName:  "电影",
		LibraryType:  "movies",
		SortOrder:    0,
	})
	harness.seedUser(t, models.User{
		Username:                           "itest_policy_history_noop_user",
		Email:                              "itest-policy-history-noop@example.com",
		EmbyID:                             "emby_user_policy",
		PlanGroup:                          stringPtr("VIP"),
		AppliedMediaLibraryTemplateVersion: 2,
	})

	recorder := harness.performAdminRequest(http.MethodPost, "/api/v1/admin/plan-groups/VIP/media-libraries/sync-apply", []byte(`{
		"libraryIds":["/data/movies"],
		"preferenceUserIds":[]
	}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Data struct {
			Status            string `json:"status"`
			AffectedUserCount int    `json:"affectedUserCount"`
			BatchID           string `json:"batchId"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.Status != "synced" || resp.Data.AffectedUserCount != 0 || resp.Data.BatchID != "" {
		t.Fatalf("unexpected noop history sync response: %+v", resp.Data)
	}

	var group models.PlanGroup
	if err := harness.database.Where("key = ?", "VIP").First(&group).Error; err != nil {
		t.Fatalf("load plan group: %v", err)
	}
	if group.MediaLibraryTemplateVersion != 2 {
		t.Fatalf("expected template version to remain 2, got %d", group.MediaLibraryTemplateVersion)
	}

	var batchCount int64
	if err := harness.database.Model(&models.EmbyPolicySyncBatch{}).Where("plan_group_key = ?", "VIP").Count(&batchCount).Error; err != nil {
		t.Fatalf("count sync batches: %v", err)
	}
	if batchCount != 0 {
		t.Fatalf("expected no sync batch for noop history sync, got %d", batchCount)
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

func TestIntegrationAdminUserMediaLibraryPolicyApplyCurrentSyncsTemplateVersion(t *testing.T) {
	harness := newIntegrationHarness(t)
	fakeEmby := newIntegrationFakeEmbyServer(t)
	harness.setSetting(t, "EMBY_URL", fakeEmby.server.URL)
	harness.setSetting(t, "EMBY_API_KEY", "integration-emby-key")

	harness.seedPlanGroup(t, models.PlanGroup{
		Key:                         "VIP",
		Name:                        "VIP",
		MediaLibraryTemplateVersion: 4,
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
		Username:                           "itest_policy_apply_admin",
		Email:                              "itest-policy-apply-admin@example.com",
		EmbyID:                             "emby_user_policy",
		PlanGroup:                          stringPtr("VIP"),
		AppliedMediaLibraryTemplateVersion: 1,
	})

	recorder := harness.performAdminRequest(http.MethodPost, "/api/v1/admin/users/"+user.ID+"/emby-policy-sync/apply-current", []byte(`{}`))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var resp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.ID != user.ID {
		t.Fatalf("expected response id=%s, got %+v", user.ID, resp.Data)
	}

	var refreshed models.User
	if err := harness.database.Where("id = ?", user.ID).First(&refreshed).Error; err != nil {
		t.Fatalf("load refreshed user: %v", err)
	}
	if refreshed.AppliedMediaLibraryTemplateVersion != 4 {
		t.Fatalf("expected applied template version 4, got %d", refreshed.AppliedMediaLibraryTemplateVersion)
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
