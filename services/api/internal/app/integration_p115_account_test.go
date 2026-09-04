package app

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/models"
)

type integrationP115ValidationOutcome struct {
	identity p115integration.AccountIdentity
	err      error
}

type integrationFakeP115Validator struct {
	t        *testing.T
	mu       sync.Mutex
	outcomes map[string][]integrationP115ValidationOutcome
	calls    []p115integration.Credential
}

type integrationBlockingP115Validator struct {
	started chan<- p115integration.Credential
	release <-chan struct{}
	outcome integrationP115ValidationOutcome
}

// ValidateCredential pauses after receiving the decrypted credential so tests can replace it concurrently.
func (v *integrationBlockingP115Validator) ValidateCredential(ctx context.Context, credential p115integration.Credential) (p115integration.AccountIdentity, error) {
	select {
	case v.started <- credential:
	case <-ctx.Done():
		return p115integration.AccountIdentity{}, ctx.Err()
	}

	select {
	case <-v.release:
		return v.outcome.identity, v.outcome.err
	case <-ctx.Done():
		return p115integration.AccountIdentity{}, ctx.Err()
	}
}

// ValidateCredential returns configured account identities without contacting 115.
func (v *integrationFakeP115Validator) ValidateCredential(_ context.Context, credential p115integration.Credential) (p115integration.AccountIdentity, error) {
	v.t.Helper()
	v.mu.Lock()
	defer v.mu.Unlock()

	v.calls = append(v.calls, credential)
	outcomes := v.outcomes[credential.Cookie]
	if len(outcomes) == 0 {
		v.t.Fatalf("unexpected 115 credential validation: accountId=%s", credential.AccountID)
	}
	outcome := outcomes[0]
	v.outcomes[credential.Cookie] = outcomes[1:]
	return outcome.identity, outcome.err
}

// ResolveDirectoryByPath returns a deterministic fake directory without contacting 115.
func (v *integrationFakeP115Validator) ResolveDirectoryByPath(_ context.Context, _ p115integration.Credential, query p115integration.DirectoryPathQuery) (*p115integration.Directory, error) {
	path := "/" + strings.TrimPrefix(strings.TrimSpace(query.RelativePath), "/")
	return &p115integration.Directory{ID: "200", ParentID: "0", Name: "Playback", Path: path}, nil
}

type integrationP115AccountResponse struct {
	ID               string                   `json:"id"`
	Role             models.P115AccountRole   `json:"role"`
	Alias            string                   `json:"alias"`
	AuthMode         models.P115AuthMode      `json:"authMode"`
	ProviderUserID   *string                  `json:"providerUserId"`
	AppType          string                   `json:"appType"`
	UserAgent        string                   `json:"userAgent"`
	TargetParentID   *string                  `json:"targetParentId"`
	Status           models.P115AccountStatus `json:"status"`
	Enabled          bool                     `json:"enabled"`
	LastValidatedAt  *time.Time               `json:"lastValidatedAt"`
	LastSucceededAt  *time.Time               `json:"lastSucceededAt"`
	LastErrorCode    *string                  `json:"lastErrorCode"`
	LastErrorMessage *string                  `json:"lastErrorMessage"`
}

func TestIntegrationP115AccountLifecycle(t *testing.T) {
	const oldCookie = "UID=source-old_A1; CID=old-cid; SEID=old-seid"
	const newCookie = "UID=source-new_F1; CID=new-cid; SEID=new-seid"
	validator := &integrationFakeP115Validator{
		t: t,
		outcomes: map[string][]integrationP115ValidationOutcome{
			oldCookie: {{identity: p115integration.AccountIdentity{ProviderUserID: "115-source-old"}}},
			newCookie: {{identity: p115integration.AccountIdentity{ProviderUserID: "115-source-new"}}},
		},
	}
	harness := newIntegrationHarnessWithP115Validator(t, validator)

	created := createIntegrationP115Account(t, harness, `{
		"role":"source",
		"alias":"source-account",
		"cookie":"`+oldCookie+`",
		"userAgent":"Ember Integration Test",
		"embyPathPrefix":"/mnt/cloudNAS/115lifetime",
		"sourceRootId":"0"
	}`)
	if created.Role != models.P115AccountRoleSource || created.AuthMode != models.P115AuthModeLegacyCookie ||
		created.Status != models.P115AccountStatusPending || created.Enabled || created.ProviderUserID != nil || created.AppType != "web" {
		t.Fatalf("unexpected created account: %+v", created)
	}

	var stored models.P115Account
	if err := harness.database.Where("id = ?", created.ID).First(&stored).Error; err != nil {
		t.Fatalf("load created 115 account: %v", err)
	}
	if stored.CookieCiphertext == nil || *stored.CookieCiphertext == oldCookie || strings.Contains(*stored.CookieCiphertext, oldCookie) {
		t.Fatal("115 Cookie was persisted as plaintext")
	}

	validated := validateIntegrationP115Account(t, harness, created.ID, http.StatusOK)
	if !validated.Valid || validated.Account.Status != models.P115AccountStatusActive || validated.Account.Enabled ||
		validated.Account.ProviderUserID == nil || *validated.Account.ProviderUserID != "115-source-old" ||
		validated.Account.LastValidatedAt == nil || validated.Account.LastSucceededAt == nil {
		t.Fatalf("unexpected successful validation: %+v", validated)
	}
	if len(validator.calls) != 1 || validator.calls[0].Cookie != oldCookie ||
		validator.calls[0].AppType != "web" || validator.calls[0].UserAgent != "Ember Integration Test" {
		t.Fatalf("unexpected validator credential: %+v", validator.calls)
	}

	enabled := setIntegrationP115AccountEnabled(t, harness, created.ID, true, http.StatusOK)
	if !enabled.Enabled || enabled.Status != models.P115AccountStatusActive {
		t.Fatalf("unexpected enabled account: %+v", enabled)
	}
	disabled := setIntegrationP115AccountEnabled(t, harness, created.ID, false, http.StatusOK)
	if disabled.Enabled {
		t.Fatalf("explicit false did not disable account: %+v", disabled)
	}

	replace := harness.performAdminRequest(http.MethodPut, "/api/v1/admin/p115-accounts/"+created.ID+"/cookie", []byte(`{"cookie":"`+newCookie+`"}`))
	assertIntegrationHTTPStatus(t, replace.Code, http.StatusOK, replace.Body.String())
	replaced := decodeIntegrationP115Account(t, replace.Body.Bytes())
	assertNoP115CredentialFields(t, replace.Body.Bytes())
	if replaced.Status != models.P115AccountStatusPending || replaced.Enabled || replaced.ProviderUserID != nil || replaced.AppType != "android" ||
		replaced.LastValidatedAt != nil || replaced.LastSucceededAt != nil || replaced.LastErrorCode != nil {
		t.Fatalf("replacement did not reset validation state: %+v", replaced)
	}
	if err := harness.database.Where("id = ?", created.ID).First(&stored).Error; err != nil {
		t.Fatalf("load replaced 115 account: %v", err)
	}
	if stored.AppType == nil || *stored.AppType != "android" {
		t.Fatalf("stored app type after replacement = %v, want android", stored.AppType)
	}

	revalidated := validateIntegrationP115Account(t, harness, created.ID, http.StatusOK)
	if !revalidated.Valid || revalidated.Account.ProviderUserID == nil || *revalidated.Account.ProviderUserID != "115-source-new" {
		t.Fatalf("unexpected replacement validation: %+v", revalidated)
	}

	list := harness.performAdminRequest(http.MethodGet, "/api/v1/admin/p115-accounts", nil)
	assertIntegrationHTTPStatus(t, list.Code, http.StatusOK, list.Body.String())
	assertNoP115CredentialFields(t, list.Body.Bytes())
	var listResp struct {
		Data []integrationP115AccountResponse `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode account list: %v", err)
	}
	if len(listResp.Data) != 1 || listResp.Data[0].ID != created.ID {
		t.Fatalf("unexpected account list: %+v", listResp.Data)
	}
}

func TestIntegrationP115SourceLocationUpdate(t *testing.T) {
	harness := newIntegrationHarnessWithP115Validator(t, &integrationFakeP115Validator{t: t})
	source := createIntegrationP115Account(t, harness, `{
		"role":"source",
		"alias":"source-location",
		"cookie":"source-location-cookie",
		"appType":"web",
		"userAgent":"itest",
		"embyPathPrefix":"/mnt/old-source",
		"sourceRootId":"0"
	}`)

	updated := harness.performAdminRequest(http.MethodPut,
		"/api/v1/admin/p115-accounts/"+source.ID+"/source-location",
		[]byte(`{"embyPathPrefix":"/mnt/cloudNAS/115lifetime","sourceRootId":"0"}`))
	assertIntegrationHTTPStatus(t, updated.Code, http.StatusOK, updated.Body.String())
	var summary struct {
		EmbyPathPrefix *string `json:"embyPathPrefix"`
		SourceRootID   *string `json:"sourceRootId"`
	}
	if err := json.Unmarshal(updated.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode source location response: %v", err)
	}
	if summary.EmbyPathPrefix == nil || *summary.EmbyPathPrefix != "/mnt/cloudNAS/115lifetime" ||
		summary.SourceRootID == nil || *summary.SourceRootID != "0" {
		t.Fatalf("source location summary = %+v", summary)
	}

	playback := createIntegrationP115Account(t, harness, `{
		"role":"playback","alias":"playback-location","cookie":"playback-location-cookie",
		"appType":"web","userAgent":"itest","targetParentId":"200"
	}`)
	rejected := harness.performAdminRequest(http.MethodPut,
		"/api/v1/admin/p115-accounts/"+playback.ID+"/source-location",
		[]byte(`{"embyPathPrefix":"/mnt/invalid","sourceRootId":"0"}`))
	assertIntegrationHTTPStatus(t, rejected.Code, http.StatusBadRequest, rejected.Body.String())
}

func TestIntegrationP115AccountEnableConstraints(t *testing.T) {
	validator := &integrationFakeP115Validator{
		t: t,
		outcomes: map[string][]integrationP115ValidationOutcome{
			"source-a-cookie":   {{identity: p115integration.AccountIdentity{ProviderUserID: "provider-shared"}}},
			"source-b-cookie":   {{identity: p115integration.AccountIdentity{ProviderUserID: "provider-source-b"}}},
			"playback-c-cookie": {{identity: p115integration.AccountIdentity{ProviderUserID: "provider-shared"}}},
		},
	}
	harness := newIntegrationHarnessWithP115Validator(t, validator)

	sourceA := createIntegrationP115Account(t, harness, `{"role":"source","alias":"source-a","cookie":"source-a-cookie","appType":"web","userAgent":"itest","embyPathPrefix":"/mnt/source-a","sourceRootId":"0"}`)
	pendingConflict := harness.performAdminRequest(http.MethodPut, "/api/v1/admin/p115-accounts/"+sourceA.ID+"/enabled", []byte(`{"enabled":true}`))
	assertIntegrationHTTPStatus(t, pendingConflict.Code, http.StatusConflict, pendingConflict.Body.String())
	if !strings.Contains(pendingConflict.Body.String(), "115 账号尚未验证为有效状态") {
		t.Fatalf("unexpected pending account response: %s", pendingConflict.Body.String())
	}
	validateIntegrationP115Account(t, harness, sourceA.ID, http.StatusOK)
	setIntegrationP115AccountEnabled(t, harness, sourceA.ID, true, http.StatusOK)

	sourceB := createIntegrationP115Account(t, harness, `{"role":"source","alias":"source-b","cookie":"source-b-cookie","appType":"web","userAgent":"itest","embyPathPrefix":"/mnt/source-b","sourceRootId":"0"}`)
	validateIntegrationP115Account(t, harness, sourceB.ID, http.StatusOK)
	roleConflict := harness.performAdminRequest(http.MethodPut, "/api/v1/admin/p115-accounts/"+sourceB.ID+"/enabled", []byte(`{"enabled":true}`))
	assertIntegrationHTTPStatus(t, roleConflict.Code, http.StatusConflict, roleConflict.Body.String())
	if !strings.Contains(roleConflict.Body.String(), "该角色已有启用的 115 账号") {
		t.Fatalf("unexpected role conflict response: %s", roleConflict.Body.String())
	}

	playbackC := createIntegrationP115Account(t, harness, `{"role":"playback","alias":"playback-c","cookie":"playback-c-cookie","appType":"web","userAgent":"itest","targetParentId":"target-1"}`)
	providerConflict := harness.performAdminRequest(http.MethodPost, "/api/v1/admin/p115-accounts/"+playbackC.ID+"/validate", nil)
	assertIntegrationHTTPStatus(t, providerConflict.Code, http.StatusConflict, providerConflict.Body.String())
	assertNoP115CredentialFields(t, providerConflict.Body.Bytes())
	if !strings.Contains(providerConflict.Body.String(), "源账号和播放账号不能使用同一个 115 账号") {
		t.Fatalf("unexpected provider conflict response: %s", providerConflict.Body.String())
	}
	var rejectedPlayback models.P115Account
	if err := harness.database.Where("id = ?", playbackC.ID).First(&rejectedPlayback).Error; err != nil {
		t.Fatalf("load playback account after provider conflict: %v", err)
	}
	if rejectedPlayback.Status != models.P115AccountStatusPending || rejectedPlayback.Enabled ||
		rejectedPlayback.ProviderUserID != nil || rejectedPlayback.LastValidatedAt != nil {
		t.Fatalf("provider conflict persisted partial validation state: %+v", rejectedPlayback)
	}

	var enabledCount int64
	if err := harness.database.Model(&models.P115Account{}).Where("enabled = true").Count(&enabledCount).Error; err != nil {
		t.Fatalf("count enabled accounts: %v", err)
	}
	if enabledCount != 1 {
		t.Fatalf("expected exactly one enabled account after conflicts, got %d", enabledCount)
	}
}

func TestIntegrationP115AccountConcurrentEnableKeepsOneAccountPerRole(t *testing.T) {
	validator := &integrationFakeP115Validator{
		t: t,
		outcomes: map[string][]integrationP115ValidationOutcome{
			"concurrent-source-a": {{identity: p115integration.AccountIdentity{ProviderUserID: "provider-concurrent-a"}}},
			"concurrent-source-b": {{identity: p115integration.AccountIdentity{ProviderUserID: "provider-concurrent-b"}}},
		},
	}
	harness := newIntegrationHarnessWithP115Validator(t, validator)

	sourceA := createIntegrationP115Account(t, harness, `{"role":"source","alias":"concurrent-a","cookie":"concurrent-source-a","appType":"web","userAgent":"itest","embyPathPrefix":"/mnt/concurrent-a","sourceRootId":"0"}`)
	sourceB := createIntegrationP115Account(t, harness, `{"role":"source","alias":"concurrent-b","cookie":"concurrent-source-b","appType":"web","userAgent":"itest","embyPathPrefix":"/mnt/concurrent-b","sourceRootId":"0"}`)
	validateIntegrationP115Account(t, harness, sourceA.ID, http.StatusOK)
	validateIntegrationP115Account(t, harness, sourceB.ID, http.StatusOK)

	type enableResult struct {
		status int
		body   []byte
	}
	start := make(chan struct{})
	results := make(chan enableResult, 2)
	for _, accountID := range []string{sourceA.ID, sourceB.ID} {
		accountID := accountID
		go func() {
			<-start
			recorder := harness.performAdminRequest(http.MethodPut, "/api/v1/admin/p115-accounts/"+accountID+"/enabled", []byte(`{"enabled":true}`))
			results <- enableResult{status: recorder.Code, body: recorder.Body.Bytes()}
		}()
	}
	close(start)

	statusCounts := map[int]int{}
	for range 2 {
		result := <-results
		statusCounts[result.status]++
		assertNoP115CredentialFields(t, result.body)
		if result.status == http.StatusConflict && !strings.Contains(string(result.body), "该角色已有启用的 115 账号") {
			t.Fatalf("unexpected concurrent role conflict response: %s", result.body)
		}
	}
	if statusCounts[http.StatusOK] != 1 || statusCounts[http.StatusConflict] != 1 {
		t.Fatalf("concurrent enable statuses = %+v, want one 200 and one 409", statusCounts)
	}

	var enabledCount int64
	if err := harness.database.Model(&models.P115Account{}).
		Where("role = ? AND enabled = true", models.P115AccountRoleSource).
		Count(&enabledCount).Error; err != nil {
		t.Fatalf("count concurrently enabled source accounts: %v", err)
	}
	if enabledCount != 1 {
		t.Fatalf("concurrent enable left %d source accounts enabled, want 1", enabledCount)
	}
}

func TestIntegrationP115AccountReplacementWinsOverInFlightValidation(t *testing.T) {
	const oldCookie = "UID=concurrent-old_A1; CID=old; SEID=old"
	const newCookie = "UID=concurrent-new_A1; CID=new; SEID=new"
	started := make(chan p115integration.Credential, 1)
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	validator := &integrationBlockingP115Validator{
		started: started,
		release: release,
		outcome: integrationP115ValidationOutcome{
			identity: p115integration.AccountIdentity{ProviderUserID: "provider-stale-validation"},
		},
	}
	harness := newIntegrationHarnessWithP115Validator(t, validator)
	account := createIntegrationP115Account(t, harness, `{"role":"source","alias":"concurrent-replace","cookie":"`+oldCookie+`","appType":"web","userAgent":"itest","embyPathPrefix":"/mnt/concurrent-replace","sourceRootId":"0"}`)

	type validationHTTPResult struct {
		status int
		body   []byte
	}
	validationDone := make(chan validationHTTPResult, 1)
	go func() {
		recorder := harness.performAdminRequest(http.MethodPost, "/api/v1/admin/p115-accounts/"+account.ID+"/validate", nil)
		validationDone <- validationHTTPResult{status: recorder.Code, body: recorder.Body.Bytes()}
	}()

	select {
	case credential := <-started:
		if credential.Cookie != oldCookie || credential.AccountID != account.ID {
			t.Fatalf("in-flight validation credential = %+v, want original account and Cookie", credential)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("validation did not reach the blocking fake")
	}

	replace := harness.performAdminRequest(http.MethodPut, "/api/v1/admin/p115-accounts/"+account.ID+"/cookie", []byte(`{"cookie":"`+newCookie+`"}`))
	assertIntegrationHTTPStatus(t, replace.Code, http.StatusOK, replace.Body.String())
	assertNoP115CredentialFields(t, replace.Body.Bytes())
	close(release)

	var validation validationHTTPResult
	select {
	case validation = <-validationDone:
	case <-time.After(5 * time.Second):
		t.Fatal("in-flight validation did not complete after release")
	}
	assertIntegrationHTTPStatus(t, validation.status, http.StatusConflict, string(validation.body))
	assertNoP115CredentialFields(t, validation.body)
	if !strings.Contains(string(validation.body), "Cookie 已被替换，请重新验证") {
		t.Fatalf("unexpected stale validation response: %s", validation.body)
	}

	detail := harness.performAdminRequest(http.MethodGet, "/api/v1/admin/p115-accounts/"+account.ID, nil)
	assertIntegrationHTTPStatus(t, detail.Code, http.StatusOK, detail.Body.String())
	finalAccount := decodeIntegrationP115Account(t, detail.Body.Bytes())
	if finalAccount.Status != models.P115AccountStatusPending || finalAccount.Enabled ||
		finalAccount.ProviderUserID != nil || finalAccount.LastValidatedAt != nil || finalAccount.LastSucceededAt != nil {
		t.Fatalf("stale validation overwrote replacement state: %+v", finalAccount)
	}
}

func TestIntegrationP115AccountValidationFailuresAndAdminAPIKey(t *testing.T) {
	const rejectedCookie = "rejected-cookie"
	const unstableCookie = "unstable-cookie"
	validator := &integrationFakeP115Validator{
		t: t,
		outcomes: map[string][]integrationP115ValidationOutcome{
			rejectedCookie: {{err: p115integration.ErrCredentialRejected}},
			unstableCookie: {
				{identity: p115integration.AccountIdentity{ProviderUserID: "provider-unstable"}},
				{err: p115integration.ErrProviderUnavailable},
			},
		},
	}
	harness := newIntegrationHarnessWithP115Validator(t, validator)

	rejected := createIntegrationP115Account(t, harness, `{"role":"source","alias":"rejected","cookie":"`+rejectedCookie+`","appType":"web","userAgent":"itest","embyPathPrefix":"/mnt/rejected","sourceRootId":"0"}`)
	rejectedResult := validateIntegrationP115Account(t, harness, rejected.ID, http.StatusOK)
	if rejectedResult.Valid || rejectedResult.Account.Status != models.P115AccountStatusExpired ||
		rejectedResult.Account.Enabled || rejectedResult.Account.LastErrorCode == nil ||
		*rejectedResult.Account.LastErrorCode != "credential_rejected" {
		t.Fatalf("unexpected rejected validation state: %+v", rejectedResult)
	}

	unstable := createIntegrationP115Account(t, harness, `{"role":"playback","alias":"unstable","cookie":"`+unstableCookie+`","appType":"web","userAgent":"itest","targetParentId":"target-2"}`)
	validateIntegrationP115Account(t, harness, unstable.ID, http.StatusOK)
	configureIntegrationP115Playback(t, harness, unstable.ID, "/Playback", 3)
	setIntegrationP115AccountEnabled(t, harness, unstable.ID, true, http.StatusOK)
	validateIntegrationP115Account(t, harness, unstable.ID, http.StatusBadGateway)

	detail := harness.performAdminRequest(http.MethodGet, "/api/v1/admin/p115-accounts/"+unstable.ID, nil)
	assertIntegrationHTTPStatus(t, detail.Code, http.StatusOK, detail.Body.String())
	assertNoP115CredentialFields(t, detail.Body.Bytes())
	unstableAfterError := decodeIntegrationP115Account(t, detail.Body.Bytes())
	if unstableAfterError.Status != models.P115AccountStatusError || !unstableAfterError.Enabled ||
		unstableAfterError.LastErrorCode == nil || *unstableAfterError.LastErrorCode != "provider_unavailable" {
		t.Fatalf("provider error did not preserve enable intent: %+v", unstableAfterError)
	}

	generated := harness.performAdminRequest(http.MethodPost, "/api/v1/admin/external-api-key", nil)
	assertIntegrationHTTPStatus(t, generated.Code, http.StatusOK, generated.Body.String())
	var generatedResp struct {
		Data struct {
			APIKey string `json:"apiKey"`
		} `json:"data"`
	}
	if err := json.Unmarshal(generated.Body.Bytes(), &generatedResp); err != nil {
		t.Fatalf("decode generated API key: %v", err)
	}
	if generatedResp.Data.APIKey == "" {
		t.Fatal("generated API key is empty")
	}
	forbidden := harness.performAuthenticatedRequest(http.MethodGet, "/api/v1/admin/p115-accounts", nil, generatedResp.Data.APIKey)
	assertIntegrationHTTPStatus(t, forbidden.Code, http.StatusForbidden, forbidden.Body.String())
	if !strings.Contains(forbidden.Body.String(), "API Key 不能管理 115 账号") {
		t.Fatalf("unexpected API key rejection: %s", forbidden.Body.String())
	}
}

func createIntegrationP115Account(t *testing.T, harness *integrationHarness, body string) integrationP115AccountResponse {
	t.Helper()
	recorder := harness.performAdminRequest(http.MethodPost, "/api/v1/admin/p115-accounts", []byte(body))
	assertIntegrationHTTPStatus(t, recorder.Code, http.StatusCreated, recorder.Body.String())
	assertNoP115CredentialFields(t, recorder.Body.Bytes())
	account := decodeIntegrationP115Account(t, recorder.Body.Bytes())
	if account.ID == "" {
		t.Fatal("created 115 account has empty id")
	}
	return account
}

func validateIntegrationP115Account(t *testing.T, harness *integrationHarness, accountID string, wantStatus int) struct {
	Valid   bool                           `json:"valid"`
	Account integrationP115AccountResponse `json:"account"`
} {
	t.Helper()
	recorder := harness.performAdminRequest(http.MethodPost, "/api/v1/admin/p115-accounts/"+accountID+"/validate", nil)
	assertIntegrationHTTPStatus(t, recorder.Code, wantStatus, recorder.Body.String())
	assertNoP115CredentialFields(t, recorder.Body.Bytes())
	var result struct {
		Valid   bool                           `json:"valid"`
		Account integrationP115AccountResponse `json:"account"`
	}
	if wantStatus == http.StatusOK {
		if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
			t.Fatalf("decode validation response: %v", err)
		}
	}
	return result
}

func setIntegrationP115AccountEnabled(t *testing.T, harness *integrationHarness, accountID string, enabled bool, wantStatus int) integrationP115AccountResponse {
	t.Helper()
	body := []byte(`{"enabled":true}`)
	if !enabled {
		body = []byte(`{"enabled":false}`)
	}
	recorder := harness.performAdminRequest(http.MethodPut, "/api/v1/admin/p115-accounts/"+accountID+"/enabled", body)
	assertIntegrationHTTPStatus(t, recorder.Code, wantStatus, recorder.Body.String())
	assertNoP115CredentialFields(t, recorder.Body.Bytes())
	if wantStatus != http.StatusOK {
		return integrationP115AccountResponse{}
	}
	return decodeIntegrationP115Account(t, recorder.Body.Bytes())
}

func configureIntegrationP115Playback(t *testing.T, harness *integrationHarness, accountID, path string, maxConcurrentStreams int) integrationP115AccountResponse {
	t.Helper()
	body, err := json.Marshal(map[string]interface{}{
		"targetParentPath":     path,
		"maxConcurrentStreams": maxConcurrentStreams,
	})
	if err != nil {
		t.Fatalf("marshal playback config: %v", err)
	}
	recorder := harness.performAdminRequest(http.MethodPut, "/api/v1/admin/p115-accounts/"+accountID+"/playback-config", body)
	assertIntegrationHTTPStatus(t, recorder.Code, http.StatusOK, recorder.Body.String())
	return decodeIntegrationP115Account(t, recorder.Body.Bytes())
}

func decodeIntegrationP115Account(t *testing.T, body []byte) integrationP115AccountResponse {
	t.Helper()
	var account integrationP115AccountResponse
	if err := json.Unmarshal(body, &account); err != nil {
		t.Fatalf("decode 115 account response: %v", err)
	}
	return account
}

func assertNoP115CredentialFields(t *testing.T, body []byte) {
	t.Helper()
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("decode response for credential check: %v", err)
	}
	var visit func(any)
	visit = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				if strings.Contains(strings.ToLower(key), "cookie") {
					t.Fatalf("response exposed credential field %q: %s", key, string(body))
				}
				visit(child)
			}
		case []any:
			for _, child := range typed {
				visit(child)
			}
		}
	}
	visit(value)
}

func assertIntegrationHTTPStatus(t *testing.T, got, want int, body string) {
	t.Helper()
	if got != want {
		t.Fatalf("expected HTTP %d, got %d body=%s", want, got, body)
	}
}

var _ p115integration.CredentialValidator = (*integrationFakeP115Validator)(nil)
var _ p115integration.CredentialValidator = (*integrationBlockingP115Validator)(nil)
