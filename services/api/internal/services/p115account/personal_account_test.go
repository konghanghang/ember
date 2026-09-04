package p115account

import (
	"context"
	"errors"
	"testing"

	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/models"
)

func TestServiceCreatePersonalAccountUsesOnlyCookieDerivedMetadata(t *testing.T) {
	store := &fakeAccountStore{}
	validator := &fakeCredentialValidator{}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})
	service.validator = validator

	result, err := service.CreatePersonalAccount(context.Background(), " user-1 ", " UID=100_Z9_1700000000; CID=fake ")
	if err != nil {
		t.Fatalf("CreatePersonalAccount() error = %v", err)
	}
	if store.created == nil || store.created.OwnerUserID == nil || *store.created.OwnerUserID != "user-1" {
		t.Fatalf("created owner = %+v", store.created)
	}
	if store.created.Role != models.P115AccountRolePlayback || store.created.Alias != personalPlaybackAlias ||
		stringValue(store.created.AppType) != "unknown" || stringValue(store.created.UserAgent) != personalProviderUserAgent {
		t.Fatalf("created metadata = %+v", store.created)
	}
	if store.created.Status != models.P115AccountStatusPending || store.created.Enabled || store.created.ProviderUserID != nil ||
		store.created.TargetParentID != nil || store.created.TargetParentPath != nil || store.created.MaxConcurrentStreams != nil {
		t.Fatalf("created state = %+v", store.created)
	}
	if validator.credential.Cookie != "" {
		t.Fatalf("create called external validator with %+v", validator.credential)
	}
	if result.AppType != "unknown" {
		t.Fatalf("personal summary exposed wrong metadata: %+v", result)
	}
}

func TestServiceCreatePersonalAccountRejectsInvalidUIDBeforePersistence(t *testing.T) {
	tests := []string{
		"CID=fake",
		"UID=0_F1_1700000000",
		"UID=100",
		"UID=100_F1_1700000000; UID=200_F1_1700000000",
	}
	for _, cookie := range tests {
		t.Run(cookie, func(t *testing.T) {
			store := &fakeAccountStore{}
			service := newServiceWithDependencies(store, fakeCredentialCipher{})
			if _, err := service.CreatePersonalAccount(context.Background(), "user-1", cookie); err == nil {
				t.Fatal("CreatePersonalAccount() error = nil")
			}
			if store.createCallCount != 0 {
				t.Fatal("invalid Cookie reached persistence")
			}
		})
	}
}

func TestServiceReplacePersonalCookieClearsProviderAndDirectoryButKeepsLimit(t *testing.T) {
	providerUserID := "100"
	targetParentID := "200"
	targetParentPath := "/Old"
	maxStreams := 3
	store := &fakeAccountStore{accounts: map[string]*models.P115Account{
		"personal": {
			ID: "personal", OwnerUserID: stringPointer("user-1"), Role: models.P115AccountRolePlayback,
			CookieCiphertext: stringPointer("encrypted:old"), AppType: stringPointer("web"), UserAgent: stringPointer(personalProviderUserAgent),
			ProviderUserID: &providerUserID, TargetParentID: &targetParentID, TargetParentPath: &targetParentPath,
			MaxConcurrentStreams: &maxStreams, Status: models.P115AccountStatusActive, Enabled: true,
		},
	}}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})

	result, err := service.ReplacePersonalCookie(context.Background(), "user-1", "UID=200_F1_1700000000; CID=new")
	if err != nil {
		t.Fatalf("ReplacePersonalCookie() error = %v", err)
	}
	account := store.accounts["personal"]
	if account.ProviderUserID != nil || account.TargetParentID != nil || account.TargetParentPath != nil || account.Enabled || account.Status != models.P115AccountStatusPending {
		t.Fatalf("replacement did not reset account: %+v", account)
	}
	if account.MaxConcurrentStreams == nil || *account.MaxConcurrentStreams != 3 || stringValue(account.AppType) != "android" {
		t.Fatalf("replacement did not preserve limit/derive app type: %+v", account)
	}
	if result.MaxConcurrentStreams == nil || *result.MaxConcurrentStreams != 3 {
		t.Fatalf("replacement summary = %+v", result)
	}
}

func TestServiceValidatePersonalAccountUsesOwnedAccount(t *testing.T) {
	store := &fakeAccountStore{accounts: map[string]*models.P115Account{
		"personal": {
			ID: "personal", OwnerUserID: stringPointer("user-1"), Role: models.P115AccountRolePlayback,
			CookieCiphertext: stringPointer("encrypted:UID=100_F1_1700000000"), AppType: stringPointer("android"), UserAgent: stringPointer(personalProviderUserAgent),
			Status: models.P115AccountStatusPending,
		},
	}}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})
	service.validator = &fakeCredentialValidator{identity: p115integration.AccountIdentity{ProviderUserID: "100"}}

	result, err := service.ValidatePersonalAccount(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("ValidatePersonalAccount() error = %v", err)
	}
	if !result.Valid || result.Account.Status != models.P115AccountStatusActive || result.Account.Enabled {
		t.Fatalf("validation result = %+v", result)
	}
	if _, err := service.ValidatePersonalAccount(context.Background(), "user-2"); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("other owner validation error = %v", err)
	}
}

func TestServiceRevokePersonalAccountIsIdempotentAndKeepsNoRuntimeFields(t *testing.T) {
	store := &fakeAccountStore{accounts: map[string]*models.P115Account{
		"personal": {
			ID: "personal", OwnerUserID: stringPointer("user-1"), Role: models.P115AccountRolePlayback,
			CookieCiphertext: stringPointer("encrypted:cookie"), AppType: stringPointer("web"), UserAgent: stringPointer(personalProviderUserAgent),
			Status: models.P115AccountStatusActive, Enabled: true,
		},
	}}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})

	if err := service.RevokePersonalAccount(context.Background(), "user-1"); err != nil {
		t.Fatalf("RevokePersonalAccount() error = %v", err)
	}
	account := store.accounts["personal"]
	if account.Status != models.P115AccountStatusRevoked || account.Enabled || account.OwnerUserID != nil ||
		account.CookieCiphertext != nil || account.AppType != nil || account.UserAgent != nil {
		t.Fatalf("revoked account retained runtime fields: %+v", account)
	}
	if err := service.RevokePersonalAccount(context.Background(), "user-1"); err != nil {
		t.Fatalf("idempotent revoke error = %v", err)
	}
}
