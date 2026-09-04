package app

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/models"
	p115accountpkg "github.com/konghang/ember/backend/internal/services/p115account"
)

type integrationP115PersonalAccountResponse struct {
	ID                            string                   `json:"id"`
	ProviderUserID                *string                  `json:"providerUserId"`
	AppType                       string                   `json:"appType"`
	TargetParentPath              *string                  `json:"targetParentPath"`
	MaxConcurrentStreams          *int                     `json:"maxConcurrentStreams"`
	EffectiveMaxConcurrentStreams *int                     `json:"effectiveMaxConcurrentStreams"`
	SimultaneousStreamLimit       *int                     `json:"simultaneousStreamLimit"`
	P115PlaybackMode              models.P115PlaybackMode  `json:"p115PlaybackMode"`
	Status                        models.P115AccountStatus `json:"status"`
	Enabled                       bool                     `json:"enabled"`
	UsageAvailable                bool                     `json:"usageAvailable"`
	ReservedStreams               *int                     `json:"reservedStreams"`
	ActiveStreams                 *int                     `json:"activeStreams"`
	OccupiedStreams               *int                     `json:"occupiedStreams"`
}

func TestIntegrationP115PersonalAccountLifecycle(t *testing.T) {
	const oldCookie = "UID=100_F1_1700000000; CID=old"
	const newCookie = "UID=200_Z9_1700000000; CID=new"
	validator := &integrationFakeP115Validator{
		t: t,
		outcomes: map[string][]integrationP115ValidationOutcome{
			oldCookie: {{identity: p115integration.AccountIdentity{ProviderUserID: "100"}}},
			newCookie: {{identity: p115integration.AccountIdentity{ProviderUserID: "200"}}},
		},
	}
	harness := newIntegrationHarnessWithP115Validator(t, validator)
	user := harness.seedUser(t, models.User{Username: "p115_personal", Email: "p115-personal@example.com"})

	rejected := harness.performUserRequest(t, user, http.MethodPost, "/api/v1/user/p115-account", []byte(`{"cookie":"`+oldCookie+`","role":"source"}`))
	assertIntegrationHTTPStatus(t, rejected.Code, http.StatusBadRequest, rejected.Body.String())

	createdRecorder := harness.performUserRequest(t, user, http.MethodPost, "/api/v1/user/p115-account", []byte(`{"cookie":"`+oldCookie+`"}`))
	assertIntegrationHTTPStatus(t, createdRecorder.Code, http.StatusCreated, createdRecorder.Body.String())
	created := decodeIntegrationP115PersonalAccount(t, createdRecorder.Body.Bytes())
	assertNoPersonalP115Secrets(t, createdRecorder.Body.Bytes())
	if created.Status != models.P115AccountStatusPending || created.Enabled || created.AppType != "android" {
		t.Fatalf("created personal account = %+v", created)
	}

	duplicate := harness.performUserRequest(t, user, http.MethodPost, "/api/v1/user/p115-account", []byte(`{"cookie":"`+oldCookie+`"}`))
	assertIntegrationHTTPStatus(t, duplicate.Code, http.StatusConflict, duplicate.Body.String())

	validated := harness.performUserRequest(t, user, http.MethodPost, "/api/v1/user/p115-account/validate", nil)
	assertIntegrationHTTPStatus(t, validated.Code, http.StatusOK, validated.Body.String())
	var validation struct {
		Valid   bool                                   `json:"valid"`
		Account integrationP115PersonalAccountResponse `json:"account"`
	}
	if err := json.Unmarshal(validated.Body.Bytes(), &validation); err != nil {
		t.Fatalf("decode validation: %v", err)
	}
	if !validation.Valid || validation.Account.Status != models.P115AccountStatusActive || validation.Account.Enabled {
		t.Fatalf("validated personal account = %+v", validation)
	}

	directory := harness.performUserRequest(t, user, http.MethodPut, "/api/v1/user/p115-account/directory", []byte(`{"targetParentPath":"/Playback"}`))
	assertIntegrationHTTPStatus(t, directory.Code, http.StatusOK, directory.Body.String())
	concurrency := harness.performUserRequest(t, user, http.MethodPut, "/api/v1/user/p115-account/concurrency", []byte(`{"maxConcurrentStreams":3}`))
	assertIntegrationHTTPStatus(t, concurrency.Code, http.StatusOK, concurrency.Body.String())
	enabled := harness.performUserRequest(t, user, http.MethodPut, "/api/v1/user/p115-account/enabled", []byte(`{"enabled":true}`))
	assertIntegrationHTTPStatus(t, enabled.Code, http.StatusOK, enabled.Body.String())
	enabledAccount := decodeIntegrationP115PersonalAccount(t, enabled.Body.Bytes())
	if !enabledAccount.Enabled || enabledAccount.TargetParentPath == nil || *enabledAccount.TargetParentPath != "/Playback" ||
		enabledAccount.EffectiveMaxConcurrentStreams == nil || *enabledAccount.EffectiveMaxConcurrentStreams != 3 {
		t.Fatalf("enabled personal account = %+v", enabledAccount)
	}

	replaced := harness.performUserRequest(t, user, http.MethodPut, "/api/v1/user/p115-account/cookie", []byte(`{"cookie":"`+newCookie+`"}`))
	assertIntegrationHTTPStatus(t, replaced.Code, http.StatusOK, replaced.Body.String())
	replacedAccount := decodeIntegrationP115PersonalAccount(t, replaced.Body.Bytes())
	if replacedAccount.Status != models.P115AccountStatusPending || replacedAccount.Enabled || replacedAccount.ProviderUserID != nil ||
		replacedAccount.TargetParentPath != nil || replacedAccount.MaxConcurrentStreams == nil || *replacedAccount.MaxConcurrentStreams != 3 || replacedAccount.AppType != "unknown" {
		t.Fatalf("replaced personal account = %+v", replacedAccount)
	}

	revalidated := harness.performUserRequest(t, user, http.MethodPost, "/api/v1/user/p115-account/validate", nil)
	assertIntegrationHTTPStatus(t, revalidated.Code, http.StatusOK, revalidated.Body.String())

	revoked := harness.performUserRequest(t, user, http.MethodDelete, "/api/v1/user/p115-account", nil)
	assertIntegrationHTTPStatus(t, revoked.Code, http.StatusOK, revoked.Body.String())
	missing := harness.performUserRequest(t, user, http.MethodGet, "/api/v1/user/p115-account", nil)
	assertIntegrationHTTPStatus(t, missing.Code, http.StatusNotFound, missing.Body.String())

	var tombstone models.P115Account
	if err := harness.database.Where("id = ?", created.ID).First(&tombstone).Error; err != nil {
		t.Fatalf("load tombstone: %v", err)
	}
	if tombstone.Status != models.P115AccountStatusRevoked || tombstone.Enabled || tombstone.OwnerUserID != nil ||
		tombstone.CookieCiphertext != nil || tombstone.ProviderUserID != nil || tombstone.TargetParentID != nil ||
		tombstone.TargetParentPath != nil || tombstone.MaxConcurrentStreams != nil {
		t.Fatalf("invalid tombstone = %+v", tombstone)
	}

	recreated := harness.performUserRequest(t, user, http.MethodPost, "/api/v1/user/p115-account", []byte(`{"cookie":"`+oldCookie+`"}`))
	assertIntegrationHTTPStatus(t, recreated.Code, http.StatusCreated, recreated.Body.String())
	if recreatedAccount := decodeIntegrationP115PersonalAccount(t, recreated.Body.Bytes()); recreatedAccount.ID == created.ID {
		t.Fatalf("recreated account reused revoked id %s", created.ID)
	}
}

func TestIntegrationP115PersonalMigrationIsIdempotentAndPreservesExplicitPolicy(t *testing.T) {
	harness := newIntegrationHarnessWithP115Validator(t, &integrationFakeP115Validator{t: t})
	if err := harness.database.Exec(`INSERT INTO plan_groups (key, name) VALUES (?, ?)`, "P115_DEFAULTS", "P115 defaults").Error; err != nil {
		t.Fatalf("insert plan group with database defaults: %v", err)
	}

	var defaults models.PlanGroup
	if err := harness.database.Where("key = ?", "P115_DEFAULTS").First(&defaults).Error; err != nil {
		t.Fatalf("load defaulted plan group: %v", err)
	}
	if defaults.P115PlaybackMode != models.P115PlaybackModePersonal || defaults.P115TransferHourlyLimit != 5 || defaults.P115TransferDailyLimit != 10 {
		t.Fatalf("plan defaults = %+v", defaults)
	}
	if err := harness.database.Model(&models.PlanGroup{}).Where("key = ?", defaults.Key).Updates(map[string]any{
		"p115_playback_mode":         models.P115PlaybackModeSystem,
		"p115_transfer_hourly_limit": 7,
		"p115_transfer_daily_limit":  4,
	}).Error; err != nil {
		t.Fatalf("set explicit p115 policy: %v", err)
	}

	migrationPath := filepath.Join(integrationMigrationsDir(t), "20260903_01_p115_personal_routing_and_quotas.sql")
	migration, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read p115 migration: %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := harness.database.Exec(string(migration)).Error; err != nil {
			t.Fatalf("execute p115 migration attempt %d: %v", attempt+1, err)
		}
	}
	if err := harness.database.Where("key = ?", defaults.Key).First(&defaults).Error; err != nil {
		t.Fatalf("reload explicit p115 policy: %v", err)
	}
	if defaults.P115PlaybackMode != models.P115PlaybackModeSystem || defaults.P115TransferHourlyLimit != 7 || defaults.P115TransferDailyLimit != 4 {
		t.Fatalf("explicit plan policy overwritten by idempotent migration: %+v", defaults)
	}
}

func TestIntegrationP115PersonalConstraintsAndTombstonePreserveTransfers(t *testing.T) {
	harness := newIntegrationHarnessWithP115Validator(t, &integrationFakeP115Validator{t: t})
	owner := harness.seedUser(t, models.User{Username: "p115_owner", Email: "p115-owner@example.com"})
	now := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	personal := integrationP115Account("p115_personal_1", models.P115AccountRolePlayback, "cipher-personal", "100")
	personal.OwnerUserID = &owner.ID
	personal.ProviderUserID = nil
	personal.Status = models.P115AccountStatusPending
	if err := harness.database.Create(&personal).Error; err != nil {
		t.Fatalf("create pending personal account: %v", err)
	}

	duplicateOwner := integrationP115Account("p115_personal_2", models.P115AccountRolePlayback, "cipher-owner-duplicate", "101")
	duplicateOwner.OwnerUserID = &owner.ID
	duplicateOwner.ProviderUserID = nil
	duplicateOwner.Status = models.P115AccountStatusPending
	if err := harness.database.Create(&duplicateOwner).Error; err == nil {
		t.Fatal("second non-revoked personal account for one owner was accepted")
	}
	if err := harness.database.Model(&models.P115Account{}).Where("id = ?", personal.ID).
		Update("target_parent_path", "/half-configured").Error; err == nil {
		t.Fatal("personal target path without target id was accepted")
	}
	if err := harness.database.Model(&models.P115Account{}).Where("id = ?", personal.ID).
		Update("enabled", true).Error; err == nil {
		t.Fatal("incomplete pending personal account was enabled")
	}
	if err := harness.database.Delete(&models.User{}, "id = ?", owner.ID).Error; err == nil {
		t.Fatal("owner deletion bypassed p115_accounts ON DELETE RESTRICT")
	}

	targetID, targetPath, maxStreams, providerUID := "200", "/Playback", 3, "100"
	if err := harness.database.Model(&models.P115Account{}).Where("id = ?", personal.ID).Updates(map[string]any{
		"provider_user_id":       providerUID,
		"target_parent_id":       targetID,
		"target_parent_path":     targetPath,
		"max_concurrent_streams": maxStreams,
		"status":                 models.P115AccountStatusActive,
		"enabled":                true,
		"last_validated_at":      now,
		"last_succeeded_at":      now,
		"updated_at":             now,
	}).Error; err != nil {
		t.Fatalf("complete personal account: %v", err)
	}

	providerConflict := integrationP115Account("p115_source_conflict", models.P115AccountRoleSource, "cipher-provider-conflict", providerUID)
	embyPrefix, sourceRoot := "/Media", "0"
	providerConflict.EmbyPathPrefix = &embyPrefix
	providerConflict.SourceRootID = &sourceRoot
	providerConflict.Status = models.P115AccountStatusActive
	if err := harness.database.Create(&providerConflict).Error; err == nil {
		t.Fatal("duplicate non-revoked Provider UID was accepted")
	}

	source := integrationP115Account("p115_source_1", models.P115AccountRoleSource, "cipher-source", "200")
	source.EmbyPathPrefix = &embyPrefix
	source.SourceRootID = &sourceRoot
	source.Status = models.P115AccountStatusActive
	if err := harness.database.Create(&source).Error; err != nil {
		t.Fatalf("create transfer source account: %v", err)
	}
	shared := integrationP115Account("p115_shared_1", models.P115AccountRolePlayback, "cipher-shared", "300")
	shared.TargetParentID = &targetID
	shared.TargetParentPath = &targetPath
	shared.MaxConcurrentStreams = &maxStreams
	shared.Status = models.P115AccountStatusActive
	shared.Enabled = true
	if err := harness.database.Create(&shared).Error; err != nil {
		t.Fatalf("create enabled shared playback account: %v", err)
	}
	secondShared := integrationP115Account("p115_shared_2", models.P115AccountRolePlayback, "cipher-shared-second", "301")
	secondShared.TargetParentID = &targetID
	secondShared.TargetParentPath = &targetPath
	secondShared.MaxConcurrentStreams = &maxStreams
	secondShared.Status = models.P115AccountStatusActive
	secondShared.Enabled = true
	if err := harness.database.Create(&secondShared).Error; err == nil {
		t.Fatal("second enabled shared playback account was accepted")
	}

	errorCode := "fixture_failure"
	transfer := models.PlaybackTransferTask{
		ID: "p115_transfer_1", SourceAccountID: source.ID, PlaybackAccountID: personal.ID,
		SHA1: strings.Repeat("A", 40), Size: 1, FileName: "fixture.mkv", TargetParentID: targetID,
		Status: models.PlaybackTransferTaskStatusFailed, AttemptCount: 1, LastErrorCode: &errorCode,
		StartedAt: now, CompletedAt: &now,
	}
	if err := harness.database.Create(&transfer).Error; err != nil {
		t.Fatalf("create personal transfer provenance: %v", err)
	}

	revoker, err := p115accountpkg.NewControlPlaneRevoker(harness.database)
	if err != nil {
		t.Fatalf("NewControlPlaneRevoker(): %v", err)
	}
	if err := revoker.RevokePersonalAccount(context.Background(), owner.ID); err != nil {
		t.Fatalf("RevokePersonalAccount(): %v", err)
	}
	var tombstone models.P115Account
	if err := harness.database.Where("id = ?", personal.ID).First(&tombstone).Error; err != nil {
		t.Fatalf("load personal tombstone: %v", err)
	}
	if tombstone.Status != models.P115AccountStatusRevoked || tombstone.OwnerUserID != nil || tombstone.CookieCiphertext != nil ||
		tombstone.ProviderUserID != nil || tombstone.TargetParentID != nil || tombstone.TargetParentPath != nil || tombstone.MaxConcurrentStreams != nil {
		t.Fatalf("personal tombstone = %+v", tombstone)
	}
	var transferCount int64
	if err := harness.database.Model(&models.PlaybackTransferTask{}).Where("id = ? AND playback_account_id = ?", transfer.ID, personal.ID).
		Count(&transferCount).Error; err != nil || transferCount != 1 {
		t.Fatalf("transfer provenance count=%d error=%v", transferCount, err)
	}
	var sharedCandidateCount int64
	if err := harness.database.Model(&models.P115Account{}).
		Where("id = ? AND owner_user_id IS NULL AND status <> ?", personal.ID, models.P115AccountStatusRevoked).
		Count(&sharedCandidateCount).Error; err != nil || sharedCandidateCount != 0 {
		t.Fatalf("revoked tombstone shared candidate count=%d error=%v", sharedCandidateCount, err)
	}
	if err := harness.database.Delete(&models.User{}, "id = ?", owner.ID).Error; err != nil {
		t.Fatalf("delete owner after tombstone: %v", err)
	}

	newOwner := harness.seedUser(t, models.User{Username: "p115_rebound", Email: "p115-rebound@example.com"})
	rebound := integrationP115Account("p115_personal_3", models.P115AccountRolePlayback, "cipher-rebound", providerUID)
	rebound.OwnerUserID = &newOwner.ID
	rebound.Status = models.P115AccountStatusActive
	if err := harness.database.Create(&rebound).Error; err != nil {
		t.Fatalf("rebind revoked Provider UID to a new account: %v", err)
	}
}

func integrationP115Account(id string, role models.P115AccountRole, ciphertext, providerUID string) models.P115Account {
	appType, userAgent := "web", "integration-agent"
	account := models.P115Account{
		ID: id, Role: role, Alias: id, AuthMode: models.P115AuthModeLegacyCookie,
		CookieCiphertext: &ciphertext, AppType: &appType, UserAgent: &userAgent,
		Status: models.P115AccountStatusPending,
	}
	if providerUID != "" {
		account.ProviderUserID = &providerUID
	}
	return account
}

func decodeIntegrationP115PersonalAccount(t *testing.T, body []byte) integrationP115PersonalAccountResponse {
	t.Helper()
	var account integrationP115PersonalAccountResponse
	if err := json.Unmarshal(body, &account); err != nil {
		t.Fatalf("decode personal account: %v body=%s", err, body)
	}
	return account
}

func assertNoPersonalP115Secrets(t *testing.T, body []byte) {
	t.Helper()
	encoded := string(body)
	for _, forbidden := range []string{"cookie", "userAgent", "ownerUserId", "targetParentId", "personal-playback"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("personal account response exposed %q: %s", forbidden, encoded)
		}
	}
}
