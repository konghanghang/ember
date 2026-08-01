package p115account

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/security/secretbox"
	"gorm.io/gorm"
)

type fakeCredentialCipher struct {
	encryptErr error
	decryptErr error
}

func (c fakeCredentialCipher) Encrypt(plain string) (string, error) {
	if c.encryptErr != nil {
		return "", c.encryptErr
	}
	return "encrypted:" + plain, nil
}

func (c fakeCredentialCipher) Decrypt(ciphertext string) (string, error) {
	if c.decryptErr != nil {
		return "", c.decryptErr
	}
	return strings.TrimPrefix(ciphertext, "encrypted:"), nil
}

type fakeAccountStore struct {
	accounts                     map[string]*models.P115Account
	createErr                    error
	getErr                       error
	replaceErr                   error
	validationErr                error
	enableErr                    error
	created                      *models.P115Account
	replacement                  credentialReplacement
	replacementID                string
	validationExpectedCiphertext string
	validationAt                 time.Time
	enabledValue                 bool
	createCallCount              int
}

func TestNewServiceRequiresDatabaseAndEncryptionKey(t *testing.T) {
	validator := &fakeCredentialValidator{}
	if _, err := NewService(nil, "encryption-key", validator); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("NewService(nil) error = %v, want ErrStoreUnavailable", err)
	}
	if _, err := NewService(&gorm.DB{}, " ", validator); !errors.Is(err, secretbox.ErrKeyMissing) {
		t.Fatalf("NewService(empty key) error = %v, want ErrKeyMissing", err)
	}
	if _, err := NewService(&gorm.DB{}, "encryption-key", nil); !errors.Is(err, ErrValidatorUnavailable) {
		t.Fatalf("NewService(nil validator) error = %v, want ErrValidatorUnavailable", err)
	}
}

func (s *fakeAccountStore) Create(_ context.Context, account *models.P115Account) error {
	s.createCallCount++
	if s.createErr != nil {
		return s.createErr
	}
	copy := *account
	if copy.ID == "" {
		copy.ID = "account_1"
		account.ID = copy.ID
	}
	s.created = &copy
	if s.accounts == nil {
		s.accounts = map[string]*models.P115Account{}
	}
	s.accounts[copy.ID] = &copy
	return nil
}

func (s *fakeAccountStore) GetByID(_ context.Context, id string) (*models.P115Account, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	account, ok := s.accounts[id]
	if !ok {
		return nil, ErrAccountNotFound
	}
	copy := *account
	return &copy, nil
}

func (s *fakeAccountStore) List(_ context.Context) ([]models.P115Account, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	accounts := make([]models.P115Account, 0, len(s.accounts))
	for _, account := range s.accounts {
		accounts = append(accounts, *account)
	}
	return accounts, nil
}

func (s *fakeAccountStore) ReplaceCredential(_ context.Context, id string, replacement credentialReplacement) (*models.P115Account, error) {
	if s.replaceErr != nil {
		return nil, s.replaceErr
	}
	account, ok := s.accounts[id]
	if !ok {
		return nil, ErrAccountNotFound
	}
	s.replacementID = id
	s.replacement = replacement
	account.CookieCiphertext = replacement.CookieCiphertext
	account.ProviderUserID = nil
	account.Status = replacement.Status
	account.Enabled = replacement.Enabled
	account.LastValidatedAt = nil
	account.LastSucceededAt = nil
	account.CooldownUntil = nil
	account.LastErrorCode = nil
	account.LastErrorMessage = nil
	copy := *account
	return &copy, nil
}

func (s *fakeAccountStore) CompleteValidationSuccess(_ context.Context, id, expectedCiphertext, providerUserID string, at time.Time) (*models.P115Account, error) {
	if s.validationErr != nil {
		return nil, s.validationErr
	}
	account, err := s.accountForValidation(id, expectedCiphertext, at)
	if err != nil {
		return nil, err
	}
	account.ProviderUserID = &providerUserID
	account.Status = models.P115AccountStatusActive
	account.LastSucceededAt = &at
	account.CooldownUntil = nil
	account.LastErrorCode = nil
	account.LastErrorMessage = nil
	copy := *account
	return &copy, nil
}

func (s *fakeAccountStore) CompleteValidationRejected(_ context.Context, id, expectedCiphertext string, at time.Time) (*models.P115Account, error) {
	if s.validationErr != nil {
		return nil, s.validationErr
	}
	account, err := s.accountForValidation(id, expectedCiphertext, at)
	if err != nil {
		return nil, err
	}
	code := validationCodeRejected
	message := "115 Cookie 已失效"
	account.Status = models.P115AccountStatusExpired
	account.Enabled = false
	account.LastErrorCode = &code
	account.LastErrorMessage = &message
	copy := *account
	return &copy, nil
}

func (s *fakeAccountStore) CompleteValidationError(_ context.Context, id, expectedCiphertext, code, message string, at time.Time) (*models.P115Account, error) {
	if s.validationErr != nil {
		return nil, s.validationErr
	}
	account, err := s.accountForValidation(id, expectedCiphertext, at)
	if err != nil {
		return nil, err
	}
	account.Status = models.P115AccountStatusError
	account.LastErrorCode = &code
	account.LastErrorMessage = &message
	copy := *account
	return &copy, nil
}

func (s *fakeAccountStore) accountForValidation(id, expectedCiphertext string, at time.Time) (*models.P115Account, error) {
	account, ok := s.accounts[id]
	if !ok {
		return nil, ErrAccountNotFound
	}
	s.validationExpectedCiphertext = expectedCiphertext
	s.validationAt = at
	if account.CookieCiphertext != expectedCiphertext {
		return nil, ErrCredentialChanged
	}
	account.LastValidatedAt = &at
	return account, nil
}

func (s *fakeAccountStore) SetEnabled(_ context.Context, id string, enabled bool) (*models.P115Account, error) {
	if s.enableErr != nil {
		return nil, s.enableErr
	}
	account, ok := s.accounts[id]
	if !ok {
		return nil, ErrAccountNotFound
	}
	s.enabledValue = enabled
	account.Enabled = enabled
	copy := *account
	return &copy, nil
}

func TestServiceCreateEncryptsCookieAndReturnsSafeSummary(t *testing.T) {
	store := &fakeAccountStore{}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})

	result, err := service.Create(context.Background(), CreateAccountInput{
		Role:      models.P115AccountRoleSource,
		Alias:     "source account",
		Cookie:    "UID=fake; CID=fake",
		AppType:   "android",
		UserAgent: "test-user-agent",
	})
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if store.created == nil {
		t.Fatal("Create() did not persist an account")
	}
	if store.created.CookieCiphertext != "encrypted:UID=fake; CID=fake" {
		t.Fatalf("stored ciphertext = %q", store.created.CookieCiphertext)
	}
	if store.created.AuthMode != models.P115AuthModeLegacyCookie {
		t.Fatalf("AuthMode = %q, want legacy_cookie", store.created.AuthMode)
	}
	if store.created.Status != models.P115AccountStatusPending || store.created.Enabled {
		t.Fatalf("new account state = %q enabled=%v, want pending and disabled", store.created.Status, store.created.Enabled)
	}
	if result.ID != "account_1" || result.Role != models.P115AccountRoleSource {
		t.Fatalf("unexpected summary: %+v", result)
	}
	if _, ok := reflect.TypeOf(AccountSummary{}).FieldByName("Cookie"); ok {
		t.Fatal("AccountSummary must not expose Cookie")
	}
	if _, ok := reflect.TypeOf(AccountSummary{}).FieldByName("CookieCiphertext"); ok {
		t.Fatal("AccountSummary must not expose CookieCiphertext")
	}
}

func TestAccountSummaryUsesCamelCaseAndOmitsCredentialFields(t *testing.T) {
	payload, err := json.Marshal(AccountSummary{
		ID:       "account_1",
		AuthMode: models.P115AuthModeLegacyCookie,
	})
	if err != nil {
		t.Fatalf("json.Marshal() failed: %v", err)
	}
	encoded := string(payload)
	if !strings.Contains(encoded, `"id":"account_1"`) || !strings.Contains(encoded, `"authMode":"legacy_cookie"`) {
		t.Fatalf("AccountSummary must use camelCase JSON fields: %s", encoded)
	}
	if strings.Contains(encoded, `"cookie":`) || strings.Contains(encoded, `"cookieCiphertext":`) || strings.Contains(encoded, `"CookieCiphertext":`) {
		t.Fatalf("AccountSummary exposed credential fields: %s", encoded)
	}
}

func TestServiceCreateValidatesRoleSpecificInput(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateAccountInput
		wantErr error
	}{
		{name: "invalid role", input: validSourceInput(P115Role("invalid")), wantErr: ErrInvalidRole},
		{name: "missing alias", input: mutateSourceInput(func(in *CreateAccountInput) { in.Alias = " " }), wantErr: ErrAliasRequired},
		{name: "missing cookie", input: mutateSourceInput(func(in *CreateAccountInput) { in.Cookie = " " }), wantErr: ErrCookieRequired},
		{name: "missing app type", input: mutateSourceInput(func(in *CreateAccountInput) { in.AppType = " " }), wantErr: ErrAppTypeRequired},
		{name: "missing user agent", input: mutateSourceInput(func(in *CreateAccountInput) { in.UserAgent = " " }), wantErr: ErrUserAgentRequired},
		{name: "source target directory", input: mutateSourceInput(func(in *CreateAccountInput) { in.TargetParentID = "target" }), wantErr: ErrSourceTargetParentUnexpected},
		{name: "playback missing target directory", input: CreateAccountInput{Role: models.P115AccountRolePlayback, Alias: "playback", Cookie: "cookie", AppType: "android", UserAgent: "ua"}, wantErr: ErrPlaybackTargetParentRequired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeAccountStore{}
			service := newServiceWithDependencies(store, fakeCredentialCipher{})
			if _, err := service.Create(context.Background(), tt.input); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Create() error = %v, want %v", err, tt.wantErr)
			}
			if store.createCallCount != 0 {
				t.Fatal("invalid input must not reach the store")
			}
		})
	}
}

func TestServiceCreatePropagatesEncryptionAndStoreErrors(t *testing.T) {
	encryptErr := errors.New("encrypt failed")
	service := newServiceWithDependencies(&fakeAccountStore{}, fakeCredentialCipher{encryptErr: encryptErr})
	if _, err := service.Create(context.Background(), validSourceInput(models.P115AccountRoleSource)); !errors.Is(err, encryptErr) {
		t.Fatalf("Create() error = %v, want encryption error", err)
	}

	storeErr := errors.New("store failed")
	service = newServiceWithDependencies(&fakeAccountStore{createErr: storeErr}, fakeCredentialCipher{})
	if _, err := service.Create(context.Background(), validSourceInput(models.P115AccountRoleSource)); !errors.Is(err, storeErr) {
		t.Fatalf("Create() error = %v, want store error", err)
	}
}

func TestServiceLoadCredentialForValidationDecryptsPendingAccount(t *testing.T) {
	store := &fakeAccountStore{accounts: map[string]*models.P115Account{
		"account_1": {
			ID:               "account_1",
			Role:             models.P115AccountRolePlayback,
			CookieCiphertext: "encrypted:UID=fake; CID=fake",
			AppType:          "android",
			UserAgent:        "playback-agent",
		},
	}}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})

	credential, err := service.LoadCredentialForValidation(context.Background(), "account_1")
	if err != nil {
		t.Fatalf("LoadCredentialForValidation() failed: %v", err)
	}
	want := p115integration.Credential{
		AccountID: "account_1",
		Cookie:    "UID=fake; CID=fake",
		AppType:   "android",
		UserAgent: "playback-agent",
	}
	if !reflect.DeepEqual(credential, want) {
		t.Fatalf("LoadCredentialForValidation() = %+v, want %+v", credential, want)
	}
}

func TestServiceLoadActiveCredentialRejectsInactiveAccount(t *testing.T) {
	store := &fakeAccountStore{accounts: map[string]*models.P115Account{
		"pending": {
			ID:               "pending",
			CookieCiphertext: "encrypted:pending-cookie",
			Status:           models.P115AccountStatusPending,
		},
		"active": {
			ID:               "active",
			CookieCiphertext: "encrypted:active-cookie",
			Status:           models.P115AccountStatusActive,
			Enabled:          true,
		},
	}}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})

	if _, err := service.LoadActiveCredential(context.Background(), "pending"); !errors.Is(err, ErrAccountUnavailable) {
		t.Fatalf("LoadActiveCredential() error = %v, want ErrAccountUnavailable", err)
	}
	credential, err := service.LoadActiveCredential(context.Background(), "active")
	if err != nil {
		t.Fatalf("LoadActiveCredential() failed: %v", err)
	}
	if credential.Cookie != "active-cookie" {
		t.Fatalf("LoadActiveCredential() Cookie = %q", credential.Cookie)
	}
}

func TestServiceLoadCredentialForValidationPropagatesDecryptError(t *testing.T) {
	decryptErr := errors.New("decrypt failed")
	store := &fakeAccountStore{accounts: map[string]*models.P115Account{
		"account_1": {ID: "account_1", CookieCiphertext: "broken"},
	}}
	service := newServiceWithDependencies(store, fakeCredentialCipher{decryptErr: decryptErr})
	if _, err := service.LoadCredentialForValidation(context.Background(), "account_1"); !errors.Is(err, decryptErr) {
		t.Fatalf("LoadCredentialForValidation() error = %v, want decrypt error", err)
	}
}

func TestServiceReplaceCookieResetsValidationState(t *testing.T) {
	now := time.Now().UTC()
	providerUserID := "provider-user"
	errorCode := "credential_invalid"
	errorMessage := "old error"
	store := &fakeAccountStore{accounts: map[string]*models.P115Account{
		"account_1": {
			ID:               "account_1",
			CookieCiphertext: "encrypted:old-cookie",
			ProviderUserID:   &providerUserID,
			Status:           models.P115AccountStatusActive,
			Enabled:          true,
			LastValidatedAt:  &now,
			LastSucceededAt:  &now,
			CooldownUntil:    &now,
			LastErrorCode:    &errorCode,
			LastErrorMessage: &errorMessage,
		},
	}}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})

	result, err := service.ReplaceCookie(context.Background(), "account_1", "new-cookie")
	if err != nil {
		t.Fatalf("ReplaceCookie() failed: %v", err)
	}
	if store.replacementID != "account_1" || store.replacement.CookieCiphertext != "encrypted:new-cookie" {
		t.Fatalf("unexpected replacement: id=%q replacement=%+v", store.replacementID, store.replacement)
	}
	if store.replacement.Status != models.P115AccountStatusPending || store.replacement.Enabled {
		t.Fatalf("replacement state = %q enabled=%v, want pending and disabled", store.replacement.Status, store.replacement.Enabled)
	}
	if result.Status != models.P115AccountStatusPending || result.Enabled || result.ProviderUserID != nil {
		t.Fatalf("unexpected result after replacement: %+v", result)
	}
}

func TestServiceReplaceCookieRejectsEmptyValue(t *testing.T) {
	tests := []struct {
		name    string
		cookie  string
		wantErr error
	}{
		{name: "empty", cookie: " ", wantErr: ErrCookieRequired},
		{name: "header injection", cookie: "UID=fake\r\nX-Test: injected", wantErr: ErrCookieInvalid},
		{name: "too long", cookie: strings.Repeat("x", maxCookieLength+1), wantErr: ErrCookieInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := newServiceWithDependencies(&fakeAccountStore{}, fakeCredentialCipher{})
			if _, err := service.ReplaceCookie(context.Background(), "account_1", tt.cookie); !errors.Is(err, tt.wantErr) {
				t.Fatalf("ReplaceCookie() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func validSourceInput(role models.P115AccountRole) CreateAccountInput {
	return CreateAccountInput{
		Role:      role,
		Alias:     "source",
		Cookie:    "cookie",
		AppType:   "android",
		UserAgent: "ua",
	}
}

func mutateSourceInput(mutate func(*CreateAccountInput)) CreateAccountInput {
	input := validSourceInput(models.P115AccountRoleSource)
	mutate(&input)
	return input
}

type P115Role = models.P115AccountRole
