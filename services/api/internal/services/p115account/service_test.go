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
	decrypts   *int
}

func (c fakeCredentialCipher) Encrypt(plain string) (string, error) {
	if c.encryptErr != nil {
		return "", c.encryptErr
	}
	return "encrypted:" + plain, nil
}

func (c fakeCredentialCipher) Decrypt(ciphertext string) (string, error) {
	if c.decrypts != nil {
		*c.decrypts++
	}
	if c.decryptErr != nil {
		return "", c.decryptErr
	}
	return strings.TrimPrefix(ciphertext, "encrypted:"), nil
}

type fakeAccountStore struct {
	accounts                         map[string]*models.P115Account
	createErr                        error
	getErr                           error
	replaceErr                       error
	validationErr                    error
	enableErr                        error
	created                          *models.P115Account
	replacement                      credentialReplacement
	replacementID                    string
	validationExpectedCiphertext     string
	validationAt                     time.Time
	enabledValue                     bool
	createCallCount                  int
	sourceLocationID                 string
	sourceLocationPrefix             string
	sourceLocationRootID             string
	playbackConfigID                 string
	playbackConfigExpectedCiphertext string
	playbackConfigExpectedUpdatedAt  time.Time
	playbackConfigPath               string
	playbackConfigTargetID           string
	playbackConfigMax                int
	playbackConfigErr                error
	personalPolicy                   PersonalPlanPolicy
	personalPolicyErr                error
	personalDirectoryOwner           string
	personalDirectoryPath            string
	personalDirectoryTargetID        string
	personalDirectoryErr             error
	personalConcurrencyOwner         string
	personalConcurrencyMax           int
	personalConcurrencyErr           error
	personalEnabledOwner             string
	personalEnabledValue             bool
	personalEnableErr                error
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

func (s *fakeAccountStore) GetByOwner(_ context.Context, ownerUserID string) (*models.P115Account, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	for _, account := range s.accounts {
		if account.OwnerUserID != nil && *account.OwnerUserID == ownerUserID && account.Status != models.P115AccountStatusRevoked {
			copy := *account
			return &copy, nil
		}
	}
	return nil, ErrAccountNotFound
}

func (s *fakeAccountStore) GetPersonalPlaybackMetadata(ctx context.Context, ownerUserID string) (*models.P115Account, error) {
	return s.GetByOwner(ctx, ownerUserID)
}

func (s *fakeAccountStore) GetSharedPlaybackMetadata(_ context.Context) (*models.P115Account, error) {
	for _, account := range s.accounts {
		if account.OwnerUserID == nil && account.Role == models.P115AccountRolePlayback && account.Status != models.P115AccountStatusRevoked && account.Enabled {
			copy := *account
			return &copy, nil
		}
	}
	return nil, ErrAccountUnavailable
}

func (s *fakeAccountStore) ResolvePlaybackRouteMetadata(ctx context.Context, ownerUserID string) (*models.P115Account, PersonalPlanPolicy, error) {
	policy, err := s.GetPersonalPlanPolicy(ctx, ownerUserID)
	if err != nil {
		return nil, PersonalPlanPolicy{}, err
	}
	var account *models.P115Account
	if policy.PlaybackMode == models.P115PlaybackModePersonal {
		account, err = s.GetPersonalPlaybackMetadata(ctx, ownerUserID)
	} else {
		account, err = s.GetSharedPlaybackMetadata(ctx)
	}
	return account, policy, err
}

func (s *fakeAccountStore) AcquirePlaybackRoute(_ context.Context, route PlaybackRoute, now, probeUntil time.Time) (*models.P115Account, error) {
	account, ok := s.accounts[route.AccountID]
	if !ok || !account.UpdatedAt.Equal(route.UpdatedAt) || !account.Enabled || account.Role != models.P115AccountRolePlayback {
		return nil, ErrRuntimeStateChanged
	}
	if route.OwnerUserID == "" {
		if account.OwnerUserID != nil {
			return nil, ErrRuntimeStateChanged
		}
	} else if account.OwnerUserID == nil || *account.OwnerUserID != route.OwnerUserID {
		return nil, ErrRuntimeStateChanged
	}
	if account.Status == models.P115AccountStatusCoolingDown {
		if account.CooldownUntil == nil || account.CooldownUntil.After(now) {
			return nil, ErrAccountCoolingDown
		}
		account.CooldownUntil = &probeUntil
		account.UpdatedAt = now
	} else if account.Status != models.P115AccountStatusActive {
		return nil, ErrAccountUnavailable
	}
	copy := *account
	return &copy, nil
}

func (s *fakeAccountStore) GetPersonalPlanPolicy(_ context.Context, ownerUserID string) (PersonalPlanPolicy, error) {
	if s.personalPolicyErr != nil {
		return PersonalPlanPolicy{}, s.personalPolicyErr
	}
	_ = ownerUserID
	return s.personalPolicy, nil
}

func (s *fakeAccountStore) GetEnabledSourceLocation(_ context.Context) (*models.P115Account, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	for _, account := range s.accounts {
		if account.OwnerUserID == nil && account.Role == models.P115AccountRoleSource && account.Enabled {
			copy := *account
			return &copy, nil
		}
	}
	return nil, ErrAccountUnavailable
}

func (s *fakeAccountStore) GetActiveByRole(_ context.Context, role models.P115AccountRole) (*models.P115Account, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	for _, account := range s.accounts {
		if account.OwnerUserID == nil && account.Role == role && account.Enabled && account.Status == models.P115AccountStatusActive {
			copy := *account
			return &copy, nil
		}
	}
	return nil, ErrAccountUnavailable
}

func (s *fakeAccountStore) AcquireRuntimeByRole(_ context.Context, role models.P115AccountRole, _, _ time.Time) (*models.P115Account, error) {
	return s.GetActiveByRole(context.Background(), role)
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
	account.CookieCiphertext = stringPointer(replacement.CookieCiphertext)
	account.AppType = stringPointer(replacement.AppType)
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

func (s *fakeAccountStore) ReplacePersonalCredential(_ context.Context, ownerUserID string, replacement credentialReplacement) (*models.P115Account, error) {
	account, err := s.GetByOwner(context.Background(), ownerUserID)
	if err != nil {
		return nil, err
	}
	stored := s.accounts[account.ID]
	stored.CookieCiphertext = stringPointer(replacement.CookieCiphertext)
	stored.AppType = stringPointer(replacement.AppType)
	stored.UserAgent = stringPointer(personalProviderUserAgent)
	stored.ProviderUserID = nil
	stored.TargetParentID = nil
	stored.TargetParentPath = nil
	stored.Status = replacement.Status
	stored.Enabled = replacement.Enabled
	stored.LastValidatedAt = nil
	stored.LastSucceededAt = nil
	stored.CooldownUntil = nil
	stored.LastErrorCode = nil
	stored.LastErrorMessage = nil
	copy := *stored
	return &copy, nil
}

func (s *fakeAccountStore) RevokePersonal(_ context.Context, ownerUserID string) error {
	for _, account := range s.accounts {
		if account.OwnerUserID == nil || *account.OwnerUserID != ownerUserID || account.Status == models.P115AccountStatusRevoked {
			continue
		}
		account.OwnerUserID = nil
		account.ProviderUserID = nil
		account.CookieCiphertext = nil
		account.AppType = nil
		account.UserAgent = nil
		account.EmbyPathPrefix = nil
		account.SourceRootID = nil
		account.TargetParentID = nil
		account.TargetParentPath = nil
		account.MaxConcurrentStreams = nil
		account.Status = models.P115AccountStatusRevoked
		account.Enabled = false
		account.LastValidatedAt = nil
		account.LastSucceededAt = nil
		account.CooldownUntil = nil
		account.LastErrorCode = nil
		account.LastErrorMessage = nil
		return nil
	}
	return nil
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

func (s *fakeAccountStore) CompleteRuntimeHealth(_ context.Context, ref runtimeCredentialRef, mutation runtimeHealthMutation) error {
	account, ok := s.accounts[ref.accountID]
	if !ok {
		return ErrAccountNotFound
	}
	if stringValue(account.CookieCiphertext) != ref.expectedCiphertext || !account.UpdatedAt.Equal(ref.expectedUpdatedAt) {
		return ErrRuntimeStateChanged
	}
	account.Status = mutation.Status
	account.CooldownUntil = mutation.CooldownUntil
	account.LastErrorCode = nil
	account.LastErrorMessage = nil
	account.UpdatedAt = mutation.At
	if mutation.Disable {
		account.Enabled = false
	}
	if mutation.Succeeded {
		account.LastSucceededAt = &mutation.At
	}
	if mutation.Code != "" {
		code := mutation.Code
		message := mutation.Message
		account.LastErrorCode = &code
		account.LastErrorMessage = &message
	}
	return nil
}

func (s *fakeAccountStore) accountForValidation(id, expectedCiphertext string, at time.Time) (*models.P115Account, error) {
	account, ok := s.accounts[id]
	if !ok {
		return nil, ErrAccountNotFound
	}
	s.validationExpectedCiphertext = expectedCiphertext
	s.validationAt = at
	if stringValue(account.CookieCiphertext) != expectedCiphertext {
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

func (s *fakeAccountStore) UpdateSourceLocation(_ context.Context, id, embyPathPrefix, sourceRootID string) (*models.P115Account, error) {
	account, ok := s.accounts[id]
	if !ok {
		return nil, ErrAccountNotFound
	}
	if account.Role != models.P115AccountRoleSource {
		return nil, ErrSourceLocationOnly
	}
	s.sourceLocationID = id
	s.sourceLocationPrefix = embyPathPrefix
	s.sourceLocationRootID = sourceRootID
	account.EmbyPathPrefix = &embyPathPrefix
	account.SourceRootID = &sourceRootID
	copy := *account
	return &copy, nil
}

func (s *fakeAccountStore) UpdatePlaybackConfig(
	_ context.Context,
	id, expectedCiphertext string,
	expectedUpdatedAt time.Time,
	targetParentPath, targetParentID string,
	maxConcurrentStreams int,
) (*models.P115Account, error) {
	s.playbackConfigID = id
	s.playbackConfigExpectedCiphertext = expectedCiphertext
	s.playbackConfigExpectedUpdatedAt = expectedUpdatedAt
	if s.playbackConfigErr != nil {
		return nil, s.playbackConfigErr
	}
	account, ok := s.accounts[id]
	if !ok {
		return nil, ErrAccountNotFound
	}
	s.playbackConfigPath = targetParentPath
	s.playbackConfigTargetID = targetParentID
	s.playbackConfigMax = maxConcurrentStreams
	account.TargetParentPath = stringPointer(targetParentPath)
	account.TargetParentID = stringPointer(targetParentID)
	account.MaxConcurrentStreams = &maxConcurrentStreams
	copy := *account
	return &copy, nil
}

func (s *fakeAccountStore) UpdatePersonalDirectory(
	_ context.Context,
	ownerUserID, expectedCiphertext string,
	expectedUpdatedAt time.Time,
	targetParentPath, targetParentID string,
) (*models.P115Account, error) {
	s.personalDirectoryOwner = ownerUserID
	if s.personalDirectoryErr != nil {
		return nil, s.personalDirectoryErr
	}
	account, err := s.GetByOwner(context.Background(), ownerUserID)
	if err != nil {
		return nil, err
	}
	if stringValue(account.CookieCiphertext) != expectedCiphertext || !account.UpdatedAt.Equal(expectedUpdatedAt) {
		return nil, ErrRuntimeStateChanged
	}
	s.personalDirectoryPath = targetParentPath
	s.personalDirectoryTargetID = targetParentID
	stored := s.accounts[account.ID]
	stored.TargetParentPath = stringPointer(targetParentPath)
	stored.TargetParentID = stringPointer(targetParentID)
	copy := *stored
	return &copy, nil
}

func (s *fakeAccountStore) UpdatePersonalConcurrency(_ context.Context, ownerUserID string, maxConcurrentStreams int) (*models.P115Account, PersonalPlanPolicy, error) {
	if s.personalConcurrencyErr != nil {
		return nil, PersonalPlanPolicy{}, s.personalConcurrencyErr
	}
	if s.personalPolicyErr != nil {
		return nil, PersonalPlanPolicy{}, s.personalPolicyErr
	}
	account, err := s.GetByOwner(context.Background(), ownerUserID)
	if err != nil {
		return nil, PersonalPlanPolicy{}, err
	}
	if _, err := effectivePersonalConcurrentLimit(maxConcurrentStreams, s.personalPolicy.SimultaneousStreamLimit); err != nil {
		return nil, PersonalPlanPolicy{}, err
	}
	s.personalConcurrencyOwner = ownerUserID
	s.personalConcurrencyMax = maxConcurrentStreams
	stored := s.accounts[account.ID]
	stored.MaxConcurrentStreams = intPointer(maxConcurrentStreams)
	copy := *stored
	return &copy, s.personalPolicy, nil
}

func (s *fakeAccountStore) SetPersonalEnabled(_ context.Context, ownerUserID string, enabled bool) (*models.P115Account, PersonalPlanPolicy, error) {
	s.personalEnabledOwner = ownerUserID
	s.personalEnabledValue = enabled
	if s.personalEnableErr != nil {
		return nil, PersonalPlanPolicy{}, s.personalEnableErr
	}
	account, err := s.GetByOwner(context.Background(), ownerUserID)
	if err != nil {
		return nil, PersonalPlanPolicy{}, err
	}
	stored := s.accounts[account.ID]
	stored.Enabled = enabled
	copy := *stored
	return &copy, s.personalPolicy, nil
}

func TestServiceCreateEncryptsCookieAndReturnsSafeSummary(t *testing.T) {
	store := &fakeAccountStore{}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})

	result, err := service.Create(context.Background(), CreateAccountInput{
		Role:           models.P115AccountRoleSource,
		Alias:          "source account",
		Cookie:         "UID=fake; CID=fake",
		AppType:        "android",
		UserAgent:      "test-user-agent",
		EmbyPathPrefix: "/mnt/cloudNAS/115lifetime",
		SourceRootID:   "0",
	})
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if store.created == nil {
		t.Fatal("Create() did not persist an account")
	}
	if stringValue(store.created.CookieCiphertext) != "encrypted:UID=fake; CID=fake" {
		t.Fatalf("stored ciphertext = %q", stringValue(store.created.CookieCiphertext))
	}
	if store.created.AuthMode != models.P115AuthModeLegacyCookie {
		t.Fatalf("AuthMode = %q, want legacy_cookie", store.created.AuthMode)
	}
	if store.created.Status != models.P115AccountStatusPending || store.created.Enabled {
		t.Fatalf("new account state = %q enabled=%v, want pending and disabled", store.created.Status, store.created.Enabled)
	}
	if store.created.EmbyPathPrefix == nil || *store.created.EmbyPathPrefix != "/mnt/cloudNAS/115lifetime" ||
		store.created.SourceRootID == nil || *store.created.SourceRootID != "0" {
		t.Fatalf("stored source location = prefix=%v root=%v", store.created.EmbyPathPrefix, store.created.SourceRootID)
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

func TestAccountSummaryDistinguishesUnavailablePlaybackUsageFromSource(t *testing.T) {
	playbackPayload, err := json.Marshal(accountSummary(&models.P115Account{ID: "playback", Role: models.P115AccountRolePlayback}))
	if err != nil {
		t.Fatalf("marshal playback summary: %v", err)
	}
	encoded := string(playbackPayload)
	for _, fragment := range []string{`"usageAvailable":false`, `"reservedStreams":null`, `"activeStreams":null`, `"occupiedStreams":null`} {
		if !strings.Contains(encoded, fragment) {
			t.Fatalf("playback summary missing %s: %s", fragment, encoded)
		}
	}
	sourcePayload, err := json.Marshal(accountSummary(&models.P115Account{ID: "source", Role: models.P115AccountRoleSource}))
	if err != nil {
		t.Fatalf("marshal source summary: %v", err)
	}
	if strings.Contains(string(sourcePayload), "usageAvailable") || strings.Contains(string(sourcePayload), "reservedStreams") {
		t.Fatalf("source summary exposed playback usage: %s", sourcePayload)
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
		{name: "missing emby path prefix", input: mutateSourceInput(func(in *CreateAccountInput) { in.EmbyPathPrefix = " " }), wantErr: ErrEmbyPathPrefixRequired},
		{name: "relative emby path prefix", input: mutateSourceInput(func(in *CreateAccountInput) { in.EmbyPathPrefix = "mnt/media" }), wantErr: ErrEmbyPathPrefixInvalid},
		{name: "ambiguous emby path prefix", input: mutateSourceInput(func(in *CreateAccountInput) { in.EmbyPathPrefix = "/mnt//media" }), wantErr: ErrEmbyPathPrefixInvalid},
		{name: "missing source root", input: mutateSourceInput(func(in *CreateAccountInput) { in.SourceRootID = " " }), wantErr: ErrSourceRootIDRequired},
		{name: "non canonical source root", input: mutateSourceInput(func(in *CreateAccountInput) { in.SourceRootID = "00" }), wantErr: ErrSourceRootIDInvalid},
		{name: "source target directory", input: mutateSourceInput(func(in *CreateAccountInput) { in.TargetParentID = "target" }), wantErr: ErrSourceTargetParentUnexpected},
		{name: "playback source location", input: CreateAccountInput{Role: models.P115AccountRolePlayback, Alias: "playback", Cookie: "cookie", AppType: "android", UserAgent: "ua", TargetParentID: "target", EmbyPathPrefix: "/mnt/media", SourceRootID: "0"}, wantErr: ErrPlaybackSourceLocationUnexpected},
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

func TestServiceCreateAllowsPlaybackConfigurationAfterValidation(t *testing.T) {
	store := &fakeAccountStore{}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})

	result, err := service.Create(context.Background(), CreateAccountInput{
		Role: models.P115AccountRolePlayback, Alias: "shared-playback", Cookie: "cookie", AppType: "web", UserAgent: "ua",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if store.created.TargetParentID != nil || store.created.TargetParentPath != nil || store.created.MaxConcurrentStreams != nil {
		t.Fatalf("pending playback account has premature config: %+v", store.created)
	}
	if result.Status != models.P115AccountStatusPending || result.Enabled {
		t.Fatalf("created playback summary = %+v", result)
	}
}

func TestServiceCreateDetectsAppTypeFromCookie(t *testing.T) {
	store := &fakeAccountStore{}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})
	input := validSourceInput(models.P115AccountRoleSource)
	input.Cookie = "UID=100_F1_1700000000; CID=fake"
	input.AppType = "ios"

	result, err := service.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if stringValue(store.created.AppType) != "android" || result.AppType != "android" {
		t.Fatalf("detected app type not persisted: stored=%q result=%q", stringValue(store.created.AppType), result.AppType)
	}
}

func TestServiceCreateUsesManualAppTypeOnlyWhenCookieTypeUnknown(t *testing.T) {
	store := &fakeAccountStore{}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})
	input := validSourceInput(models.P115AccountRoleSource)
	input.Cookie = "UID=100_A2_1700000000; CID=fake"
	input.AppType = "custom_client"

	result, err := service.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}
	if stringValue(store.created.AppType) != "custom_client" || result.AppType != "custom_client" {
		t.Fatalf("manual app type fallback not persisted: stored=%q result=%q", stringValue(store.created.AppType), result.AppType)
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
			CookieCiphertext: stringPointer("encrypted:UID=fake; CID=fake"),
			AppType:          stringPointer("android"),
			UserAgent:        stringPointer("playback-agent"),
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
			CookieCiphertext: stringPointer("encrypted:pending-cookie"),
			AppType:          stringPointer("web"),
			UserAgent:        stringPointer("fixture-agent"),
			Status:           models.P115AccountStatusPending,
		},
		"active": {
			ID:               "active",
			CookieCiphertext: stringPointer("encrypted:active-cookie"),
			AppType:          stringPointer("web"),
			UserAgent:        stringPointer("fixture-agent"),
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

func TestServiceLoadActiveCredentialByRoleReturnsProviderIdentityAndTarget(t *testing.T) {
	providerUserID := "provider-playback"
	targetParentID := "200000002"
	store := &fakeAccountStore{accounts: map[string]*models.P115Account{
		"playback": {
			ID:               "playback",
			Role:             models.P115AccountRolePlayback,
			ProviderUserID:   &providerUserID,
			CookieCiphertext: stringPointer("encrypted:playback-cookie"),
			AppType:          stringPointer("ios"),
			UserAgent:        stringPointer("playback-agent"),
			TargetParentID:   &targetParentID,
			Status:           models.P115AccountStatusActive,
			Enabled:          true,
		},
	}}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})

	active, err := service.LoadActiveCredentialByRole(context.Background(), models.P115AccountRolePlayback)
	if err != nil {
		t.Fatalf("LoadActiveCredentialByRole() error = %v", err)
	}
	if active.Role != models.P115AccountRolePlayback || active.ProviderUserID != providerUserID ||
		active.TargetParentID != targetParentID || active.Credential.AccountID != "playback" ||
		active.Credential.Cookie != "playback-cookie" {
		t.Fatalf("LoadActiveCredentialByRole() = %+v", active)
	}
}

func TestServiceLoadActiveSourceCredentialRequiresLocation(t *testing.T) {
	providerUserID := "provider-source"
	embyPathPrefix := "/mnt/cloudNAS/115lifetime"
	sourceRootID := "0"
	store := &fakeAccountStore{accounts: map[string]*models.P115Account{
		"source": {
			ID: "source", Role: models.P115AccountRoleSource, ProviderUserID: &providerUserID,
			CookieCiphertext: stringPointer("encrypted:source-cookie"), AppType: stringPointer("web"), UserAgent: stringPointer("fixture-agent"), EmbyPathPrefix: &embyPathPrefix,
			SourceRootID: &sourceRootID, Status: models.P115AccountStatusActive, Enabled: true,
		},
		"missing": {
			ID: "missing", Role: models.P115AccountRoleSource, ProviderUserID: &providerUserID,
			CookieCiphertext: stringPointer("encrypted:source-cookie"), AppType: stringPointer("web"), UserAgent: stringPointer("fixture-agent"), Status: models.P115AccountStatusActive, Enabled: false,
		},
	}}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})

	active, err := service.LoadActiveCredentialByRole(context.Background(), models.P115AccountRoleSource)
	if err != nil {
		t.Fatalf("LoadActiveCredentialByRole(source) error = %v", err)
	}
	if active.EmbyPathPrefix != embyPathPrefix || active.SourceRootID != sourceRootID {
		t.Fatalf("active source location = %+v", active)
	}

	store.accounts["source"].Enabled = false
	store.accounts["missing"].Enabled = true
	if _, err := service.LoadActiveCredentialByRole(context.Background(), models.P115AccountRoleSource); !errors.Is(err, ErrAccountUnavailable) {
		t.Fatalf("LoadActiveCredentialByRole(missing location) error = %v, want ErrAccountUnavailable", err)
	}
}

func TestServiceLoadCredentialForValidationPropagatesDecryptError(t *testing.T) {
	decryptErr := errors.New("decrypt failed")
	store := &fakeAccountStore{accounts: map[string]*models.P115Account{
		"account_1": {ID: "account_1", CookieCiphertext: stringPointer("broken"), AppType: stringPointer("web"), UserAgent: stringPointer("fixture-agent")},
	}}
	service := newServiceWithDependencies(store, fakeCredentialCipher{decryptErr: decryptErr})
	if _, err := service.LoadCredentialForValidation(context.Background(), "account_1"); !errors.Is(err, decryptErr) {
		t.Fatalf("LoadCredentialForValidation() error = %v, want decrypt error", err)
	}
}

func TestServiceLoadEnabledSourceLocationDoesNotDecryptOrGateProviderHealth(t *testing.T) {
	for _, status := range []models.P115AccountStatus{
		models.P115AccountStatusActive,
		models.P115AccountStatusError,
		models.P115AccountStatusCoolingDown,
	} {
		t.Run(string(status), func(t *testing.T) {
			embyPathPrefix := "/mnt/cloudNAS/115lifetime"
			sourceRootID := "0"
			providerUserID := "provider-secret"
			store := &fakeAccountStore{accounts: map[string]*models.P115Account{
				"source": {
					ID: "source", Role: models.P115AccountRoleSource, Enabled: true, Status: status,
					EmbyPathPrefix: &embyPathPrefix, SourceRootID: &sourceRootID,
					ProviderUserID: &providerUserID, CookieCiphertext: stringPointer("encrypted:cookie-secret"),
				},
			}}
			decrypts := 0
			service := newServiceWithDependencies(store, fakeCredentialCipher{decrypts: &decrypts})

			location, err := service.LoadEnabledSourceLocation(context.Background())
			if err != nil {
				t.Fatalf("LoadEnabledSourceLocation() error = %v", err)
			}
			if location.AccountID != "source" || location.EmbyPathPrefix != embyPathPrefix || location.SourceRootID != sourceRootID {
				t.Fatalf("LoadEnabledSourceLocation() = %+v", location)
			}
			if decrypts != 0 {
				t.Fatalf("LoadEnabledSourceLocation() decrypted credential %d times", decrypts)
			}
		})
	}
}

func TestServiceLoadEnabledSourceLocationRejectsDisabledOrInvalidSource(t *testing.T) {
	embyPathPrefix := "/mnt/cloudNAS/115lifetime"
	sourceRootID := "0"
	tests := []struct {
		name    string
		account models.P115Account
	}{
		{name: "disabled", account: models.P115Account{ID: "source", Role: models.P115AccountRoleSource, Enabled: false, EmbyPathPrefix: &embyPathPrefix, SourceRootID: &sourceRootID}},
		{name: "missing prefix", account: models.P115Account{ID: "source", Role: models.P115AccountRoleSource, Enabled: true, SourceRootID: &sourceRootID}},
		{name: "invalid root", account: models.P115Account{ID: "source", Role: models.P115AccountRoleSource, Enabled: true, EmbyPathPrefix: &embyPathPrefix, SourceRootID: func() *string { value := "01"; return &value }()}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeAccountStore{accounts: map[string]*models.P115Account{"source": &test.account}}
			service := newServiceWithDependencies(store, fakeCredentialCipher{})
			if _, err := service.LoadEnabledSourceLocation(context.Background()); !errors.Is(err, ErrAccountUnavailable) {
				t.Fatalf("LoadEnabledSourceLocation() error = %v, want ErrAccountUnavailable", err)
			}
		})
	}
}

func TestServiceUpdateSourceLocationPersistsNormalizedPair(t *testing.T) {
	store := &fakeAccountStore{accounts: map[string]*models.P115Account{
		"source":   {ID: "source", Role: models.P115AccountRoleSource},
		"playback": {ID: "playback", Role: models.P115AccountRolePlayback},
	}}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})

	result, err := service.UpdateSourceLocation(context.Background(), "source", SourceLocationInput{
		EmbyPathPrefix: " /mnt/cloudNAS/115lifetime ",
		SourceRootID:   "0",
	})
	if err != nil {
		t.Fatalf("UpdateSourceLocation() error = %v", err)
	}
	if store.sourceLocationID != "source" || store.sourceLocationPrefix != "/mnt/cloudNAS/115lifetime" ||
		store.sourceLocationRootID != "0" || result.EmbyPathPrefix == nil || result.SourceRootID == nil {
		t.Fatalf("source location result=%+v store=%q/%q/%q", result,
			store.sourceLocationID, store.sourceLocationPrefix, store.sourceLocationRootID)
	}
	if _, err := service.UpdateSourceLocation(context.Background(), "playback", SourceLocationInput{
		EmbyPathPrefix: "/mnt/media", SourceRootID: "0",
	}); !errors.Is(err, ErrSourceLocationOnly) {
		t.Fatalf("UpdateSourceLocation(playback) error = %v, want ErrSourceLocationOnly", err)
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
			CookieCiphertext: stringPointer("encrypted:old-cookie"),
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

	result, err := service.ReplaceCookie(context.Background(), "account_1", ReplaceCookieInput{
		Cookie:  "UID=100_F1_1700000000; CID=new",
		AppType: "ios",
	})
	if err != nil {
		t.Fatalf("ReplaceCookie() failed: %v", err)
	}
	if store.replacementID != "account_1" || store.replacement.CookieCiphertext != "encrypted:UID=100_F1_1700000000; CID=new" {
		t.Fatalf("unexpected replacement: id=%q replacement=%+v", store.replacementID, store.replacement)
	}
	if store.replacement.AppType != "android" || result.AppType != "android" {
		t.Fatalf("replacement app type = %q result=%q, want android", store.replacement.AppType, result.AppType)
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
			if _, err := service.ReplaceCookie(context.Background(), "account_1", ReplaceCookieInput{Cookie: tt.cookie, AppType: "web"}); !errors.Is(err, tt.wantErr) {
				t.Fatalf("ReplaceCookie() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestServiceReplaceCookieRequiresManualAppTypeForUnknownClient(t *testing.T) {
	service := newServiceWithDependencies(&fakeAccountStore{}, fakeCredentialCipher{})
	_, err := service.ReplaceCookie(context.Background(), "account_1", ReplaceCookieInput{
		Cookie: "UID=100_A2_1700000000; CID=fake",
	})
	if !errors.Is(err, ErrAppTypeRequired) {
		t.Fatalf("ReplaceCookie() error = %v, want ErrAppTypeRequired", err)
	}
}

func TestServiceReplaceCookieUsesManualAppTypeForUnknownClient(t *testing.T) {
	store := &fakeAccountStore{accounts: map[string]*models.P115Account{
		"account_1": {ID: "account_1", AppType: stringPointer("web"), UserAgent: stringPointer("fixture-agent"), CookieCiphertext: stringPointer("encrypted:old-cookie")},
	}}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})
	result, err := service.ReplaceCookie(context.Background(), "account_1", ReplaceCookieInput{
		Cookie:  "UID=100_A2_1700000000; CID=fake",
		AppType: "custom_client",
	})
	if err != nil {
		t.Fatalf("ReplaceCookie() failed: %v", err)
	}
	if store.replacement.AppType != "custom_client" || result.AppType != "custom_client" {
		t.Fatalf("manual replacement app type not persisted: replacement=%q result=%q", store.replacement.AppType, result.AppType)
	}
}

func validSourceInput(role models.P115AccountRole) CreateAccountInput {
	return CreateAccountInput{
		Role:           role,
		Alias:          "source",
		Cookie:         "cookie",
		AppType:        "android",
		UserAgent:      "ua",
		EmbyPathPrefix: "/mnt/cloudNAS/115lifetime",
		SourceRootID:   "0",
	}
}

func mutateSourceInput(mutate func(*CreateAccountInput)) CreateAccountInput {
	input := validSourceInput(models.P115AccountRoleSource)
	mutate(&input)
	return input
}

type P115Role = models.P115AccountRole
