package p115account

import (
	"bytes"
	"context"
	"errors"
	"log"
	"reflect"
	"strings"
	"testing"
	"time"

	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/models"
)

type fakeCredentialValidator struct {
	identity   p115integration.AccountIdentity
	err        error
	credential p115integration.Credential
}

func (v *fakeCredentialValidator) ValidateCredential(_ context.Context, credential p115integration.Credential) (p115integration.AccountIdentity, error) {
	v.credential = credential
	return v.identity, v.err
}

func TestServiceListAndGetReturnSafeAccountSummaries(t *testing.T) {
	store := &fakeAccountStore{accounts: map[string]*models.P115Account{
		"account_1": {
			ID:               "account_1",
			Role:             models.P115AccountRoleSource,
			Alias:            "source",
			CookieCiphertext: stringPointer("encrypted:cookie-secret"),
		},
	}}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})

	items, err := service.List(context.Background())
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	if len(items) != 1 || items[0].ID != "account_1" {
		t.Fatalf("unexpected List() result: %+v", items)
	}
	item, err := service.Get(context.Background(), "account_1")
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	if item.ID != "account_1" {
		t.Fatalf("unexpected Get() result: %+v", item)
	}
	if _, ok := reflect.TypeOf(*item).FieldByName("CookieCiphertext"); ok {
		t.Fatal("account summary must not expose CookieCiphertext")
	}
}

func TestAdministratorAccountMethodsRejectPersonalAccounts(t *testing.T) {
	store := &fakeAccountStore{accounts: map[string]*models.P115Account{
		"personal": {
			ID: "personal", OwnerUserID: stringPointer("user-1"), Role: models.P115AccountRolePlayback,
			CookieCiphertext: stringPointer("encrypted:UID=100_F1_1700000000"), AppType: stringPointer("android"), UserAgent: stringPointer(personalProviderUserAgent),
			Status: models.P115AccountStatusActive,
		},
	}}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})
	service.validator = &fakeCredentialValidator{}

	if _, err := service.Get(context.Background(), "personal"); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("Get(personal) error = %v", err)
	}
	if _, err := service.ReplaceCookie(context.Background(), "personal", ReplaceCookieInput{Cookie: "UID=200_F1_1700000000"}); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("ReplaceCookie(personal) error = %v", err)
	}
	if _, err := service.Validate(context.Background(), "personal"); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("Validate(personal) error = %v", err)
	}
	if _, err := service.SetEnabled(context.Background(), "personal", false); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("SetEnabled(personal) error = %v", err)
	}
}

func TestServiceValidateMarksAccountActiveWithoutEnablingIt(t *testing.T) {
	validatedAt := time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC)
	store := &fakeAccountStore{accounts: map[string]*models.P115Account{
		"account_1": {
			ID:               "account_1",
			CookieCiphertext: stringPointer("encrypted:UID=123_A1"),
			AppType:          stringPointer("android"),
			UserAgent:        stringPointer("agent"),
			Status:           models.P115AccountStatusPending,
		},
	}}
	validator := &fakeCredentialValidator{identity: p115integration.AccountIdentity{ProviderUserID: "123"}}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})
	service.validator = validator
	service.now = func() time.Time { return validatedAt }

	result, err := service.Validate(context.Background(), "account_1")
	if err != nil {
		t.Fatalf("Validate() failed: %v", err)
	}
	if !result.Valid || result.Account.Status != models.P115AccountStatusActive || result.Account.Enabled {
		t.Fatalf("unexpected validation result: %+v", result)
	}
	if validator.credential.Cookie != "UID=123_A1" || store.validationExpectedCiphertext != "encrypted:UID=123_A1" {
		t.Fatalf("validation did not preserve credential version: credential=%+v expectedCiphertext=%q", validator.credential, store.validationExpectedCiphertext)
	}
	if store.validationAt != validatedAt {
		t.Fatalf("validation timestamp = %s, want %s", store.validationAt, validatedAt)
	}
}

func TestServiceValidatePersistsRejectedAndUnavailableStates(t *testing.T) {
	tests := []struct {
		name        string
		providerErr error
		wantStatus  models.P115AccountStatus
		wantEnabled bool
		wantValid   bool
		wantErr     error
	}{
		{name: "rejected", providerErr: p115integration.ErrCredentialRejected, wantStatus: models.P115AccountStatusExpired, wantEnabled: false, wantValid: false},
		{name: "unavailable", providerErr: p115integration.ErrProviderUnavailable, wantStatus: models.P115AccountStatusError, wantEnabled: true, wantErr: p115integration.ErrProviderUnavailable},
		{name: "protocol", providerErr: p115integration.ErrProviderProtocol, wantStatus: models.P115AccountStatusError, wantEnabled: true, wantErr: p115integration.ErrProviderProtocol},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeAccountStore{accounts: map[string]*models.P115Account{
				"account_1": {ID: "account_1", CookieCiphertext: stringPointer("encrypted:UID=123_A1"), AppType: stringPointer("web"), UserAgent: stringPointer("agent"), Enabled: true},
			}}
			service := newServiceWithDependencies(store, fakeCredentialCipher{})
			service.validator = &fakeCredentialValidator{err: tt.providerErr}

			result, err := service.Validate(context.Background(), "account_1")
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Validate() error = %v, want %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("Validate() failed: %v", err)
			}
			if store.accounts["account_1"].Status != tt.wantStatus {
				t.Fatalf("stored status = %q, want %q", store.accounts["account_1"].Status, tt.wantStatus)
			}
			if store.accounts["account_1"].Enabled != tt.wantEnabled {
				t.Fatalf("stored enabled = %t, want %t", store.accounts["account_1"].Enabled, tt.wantEnabled)
			}
			if tt.wantErr == nil && (result == nil || result.Valid != tt.wantValid) {
				t.Fatalf("unexpected validation result: %+v", result)
			}
		})
	}
}

func TestServiceValidateDoesNotLogRawProviderError(t *testing.T) {
	const providerSecret = "provider-response-secret"
	store := &fakeAccountStore{accounts: map[string]*models.P115Account{
		"account_1": {
			ID: "account_1", CookieCiphertext: stringPointer("encrypted:UID=123_A1"),
			AppType: stringPointer("web"), UserAgent: stringPointer("agent"),
			Status: models.P115AccountStatusPending,
		},
	}}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})
	service.validator = &fakeCredentialValidator{err: errors.New(providerSecret)}

	var logs bytes.Buffer
	previousWriter := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previousWriter) })

	if _, err := service.Validate(context.Background(), "account_1"); !errors.Is(err, p115integration.ErrProviderUnavailable) {
		t.Fatalf("Validate() error = %v", err)
	}
	if strings.Contains(logs.String(), providerSecret) || !strings.Contains(logs.String(), "errorType=") {
		t.Fatalf("validation logs = %q", logs.String())
	}
}

func TestValidateP115AccountEnableState(t *testing.T) {
	providerUserID := "123"
	validatedAt := time.Now().UTC()
	embyPathPrefix := "/mnt/cloudNAS/115lifetime"
	sourceRootID := "0"
	targetParentID := "200"
	targetParentPath := "/Playback"
	maxConcurrentStreams := 3
	ciphertext := "encrypted:cookie"
	appType := "web"
	userAgent := "fixture-agent"
	tests := []struct {
		name    string
		account models.P115Account
		wantErr error
	}{
		{name: "pending", account: models.P115Account{Status: models.P115AccountStatusPending, ProviderUserID: &providerUserID, LastValidatedAt: &validatedAt}, wantErr: ErrAccountUnavailable},
		{name: "missing provider user", account: models.P115Account{Status: models.P115AccountStatusActive, LastValidatedAt: &validatedAt}, wantErr: ErrAccountUnavailable},
		{name: "missing validation", account: models.P115Account{Status: models.P115AccountStatusActive, ProviderUserID: &providerUserID}, wantErr: ErrAccountUnavailable},
		{name: "source missing location", account: models.P115Account{Role: models.P115AccountRoleSource, Status: models.P115AccountStatusActive, ProviderUserID: &providerUserID, LastValidatedAt: &validatedAt}, wantErr: ErrAccountUnavailable},
		{name: "active source", account: models.P115Account{Role: models.P115AccountRoleSource, Status: models.P115AccountStatusActive, ProviderUserID: &providerUserID, LastValidatedAt: &validatedAt, CookieCiphertext: &ciphertext, AppType: &appType, UserAgent: &userAgent, EmbyPathPrefix: &embyPathPrefix, SourceRootID: &sourceRootID}},
		{name: "playback missing target", account: models.P115Account{Role: models.P115AccountRolePlayback, Status: models.P115AccountStatusActive, ProviderUserID: &providerUserID, LastValidatedAt: &validatedAt}, wantErr: ErrAccountUnavailable},
		{name: "playback missing path and limit", account: models.P115Account{Role: models.P115AccountRolePlayback, Status: models.P115AccountStatusActive, ProviderUserID: &providerUserID, LastValidatedAt: &validatedAt, TargetParentID: &targetParentID}, wantErr: ErrAccountUnavailable},
		{name: "active playback", account: models.P115Account{Role: models.P115AccountRolePlayback, Status: models.P115AccountStatusActive, ProviderUserID: &providerUserID, LastValidatedAt: &validatedAt, CookieCiphertext: &ciphertext, AppType: &appType, UserAgent: &userAgent, TargetParentID: &targetParentID, TargetParentPath: &targetParentPath, MaxConcurrentStreams: &maxConcurrentStreams}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateP115AccountEnableState(&tt.account)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("validateP115AccountEnableState() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestServiceValidateRejectsStaleCredentialResult(t *testing.T) {
	store := &fakeAccountStore{
		accounts: map[string]*models.P115Account{
			"account_1": {ID: "account_1", CookieCiphertext: stringPointer("encrypted:old-cookie"), AppType: stringPointer("web"), UserAgent: stringPointer("agent")},
		},
		validationErr: ErrCredentialChanged,
	}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})
	service.validator = &fakeCredentialValidator{identity: p115integration.AccountIdentity{ProviderUserID: "123"}}

	if _, err := service.Validate(context.Background(), "account_1"); !errors.Is(err, ErrCredentialChanged) {
		t.Fatalf("Validate() error = %v, want ErrCredentialChanged", err)
	}
}

func TestServiceSetEnabledDelegatesAtomicStateCheck(t *testing.T) {
	store := &fakeAccountStore{accounts: map[string]*models.P115Account{
		"account_1": {ID: "account_1", Status: models.P115AccountStatusActive},
	}}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})

	result, err := service.SetEnabled(context.Background(), "account_1", true)
	if err != nil {
		t.Fatalf("SetEnabled() failed: %v", err)
	}
	if !result.Enabled || !store.enabledValue {
		t.Fatalf("SetEnabled() did not enable account: result=%+v stored=%t", result, store.enabledValue)
	}

	store.enableErr = ErrAccountUnavailable
	if _, err := service.SetEnabled(context.Background(), "account_1", true); !errors.Is(err, ErrAccountUnavailable) {
		t.Fatalf("SetEnabled() error = %v, want ErrAccountUnavailable", err)
	}
}
