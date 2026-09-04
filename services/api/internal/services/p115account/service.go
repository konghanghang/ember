package p115account

import (
	"context"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/security/secretbox"
	"github.com/konghang/ember/backend/internal/services/p115quota"
	"gorm.io/gorm"
)

const credentialEncryptionPurpose = "p115-cookie"

const maxCookieLength = 16 * 1024

const maxEmbyPathPrefixLength = 4 * 1024

var appTypePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type credentialCipher interface {
	Encrypt(plain string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

type directoryResolver interface {
	ResolveDirectoryByPath(context.Context, p115integration.Credential, p115integration.DirectoryPathQuery) (*p115integration.Directory, error)
}

type accountStore interface {
	Create(ctx context.Context, account *models.P115Account) error
	List(ctx context.Context) ([]models.P115Account, error)
	GetByID(ctx context.Context, id string) (*models.P115Account, error)
	GetByOwner(ctx context.Context, ownerUserID string) (*models.P115Account, error)
	ResolvePlaybackRouteMetadata(ctx context.Context, ownerUserID string) (*models.P115Account, PersonalPlanPolicy, error)
	GetPersonalPlaybackMetadata(ctx context.Context, ownerUserID string) (*models.P115Account, error)
	GetSharedPlaybackMetadata(ctx context.Context) (*models.P115Account, error)
	GetPersonalPlanPolicy(ctx context.Context, ownerUserID string) (PersonalPlanPolicy, error)
	GetEnabledSourceLocation(ctx context.Context) (*models.P115Account, error)
	AcquireRuntimeByRole(ctx context.Context, role models.P115AccountRole, now, probeUntil time.Time) (*models.P115Account, error)
	AcquirePlaybackRoute(ctx context.Context, route PlaybackRoute, now, probeUntil time.Time) (*models.P115Account, error)
	ReplaceCredential(ctx context.Context, id string, replacement credentialReplacement) (*models.P115Account, error)
	ReplacePersonalCredential(ctx context.Context, ownerUserID string, replacement credentialReplacement) (*models.P115Account, error)
	CompleteValidationSuccess(ctx context.Context, id, expectedCiphertext, providerUserID string, at time.Time) (*models.P115Account, error)
	CompleteValidationRejected(ctx context.Context, id, expectedCiphertext string, at time.Time) (*models.P115Account, error)
	CompleteValidationError(ctx context.Context, id, expectedCiphertext, code, message string, at time.Time) (*models.P115Account, error)
	CompleteRuntimeHealth(ctx context.Context, ref runtimeCredentialRef, mutation runtimeHealthMutation) error
	SetEnabled(ctx context.Context, id string, enabled bool) (*models.P115Account, error)
	UpdateSourceLocation(ctx context.Context, id, embyPathPrefix, sourceRootID string) (*models.P115Account, error)
	UpdatePlaybackConfig(ctx context.Context, id, expectedCiphertext string, expectedUpdatedAt time.Time, targetParentPath, targetParentID string, maxConcurrentStreams int) (*models.P115Account, error)
	UpdatePersonalDirectory(ctx context.Context, ownerUserID, expectedCiphertext string, expectedUpdatedAt time.Time, targetParentPath, targetParentID string) (*models.P115Account, error)
	UpdatePersonalConcurrency(ctx context.Context, ownerUserID string, maxConcurrentStreams int) (*models.P115Account, PersonalPlanPolicy, error)
	SetPersonalEnabled(ctx context.Context, ownerUserID string, enabled bool) (*models.P115Account, PersonalPlanPolicy, error)
	RevokePersonal(ctx context.Context, ownerUserID string) error
}

type credentialReplacement struct {
	CookieCiphertext string
	AppType          string
	Status           models.P115AccountStatus
	Enabled          bool
}

// CreateAccountInput contains the administrator-managed account fields and write-only Cookie.
type CreateAccountInput struct {
	Role           models.P115AccountRole
	Alias          string
	Cookie         string
	AppType        string
	UserAgent      string
	EmbyPathPrefix string
	SourceRootID   string
	TargetParentID string
}

// AccountSummary is the safe account view and intentionally has no credential field.
type AccountSummary struct {
	ID                   string                   `json:"id"`
	Role                 models.P115AccountRole   `json:"role"`
	Alias                string                   `json:"alias"`
	AuthMode             models.P115AuthMode      `json:"authMode"`
	ProviderUserID       *string                  `json:"providerUserId,omitempty"`
	AppType              string                   `json:"appType"`
	UserAgent            string                   `json:"userAgent"`
	EmbyPathPrefix       *string                  `json:"embyPathPrefix,omitempty"`
	SourceRootID         *string                  `json:"sourceRootId,omitempty"`
	TargetParentID       *string                  `json:"targetParentId,omitempty"`
	TargetParentPath     *string                  `json:"targetParentPath,omitempty"`
	MaxConcurrentStreams *int                     `json:"maxConcurrentStreams,omitempty"`
	UsageAvailable       *bool                    `json:"usageAvailable,omitempty"`
	ReservedStreams      any                      `json:"reservedStreams,omitempty"`
	ActiveStreams        any                      `json:"activeStreams,omitempty"`
	OccupiedStreams      any                      `json:"occupiedStreams,omitempty"`
	Status               models.P115AccountStatus `json:"status"`
	Enabled              bool                     `json:"enabled"`
	LastValidatedAt      *time.Time               `json:"lastValidatedAt,omitempty"`
	LastSucceededAt      *time.Time               `json:"lastSucceededAt,omitempty"`
	CooldownUntil        *time.Time               `json:"cooldownUntil,omitempty"`
	LastErrorCode        *string                  `json:"lastErrorCode,omitempty"`
	LastErrorMessage     *string                  `json:"lastErrorMessage,omitempty"`
	CreatedAt            time.Time                `json:"createdAt"`
	UpdatedAt            time.Time                `json:"updatedAt"`
}

// ValidationResult reports a completed credential check without exposing credential data.
type ValidationResult struct {
	Valid   bool            `json:"valid"`
	Account *AccountSummary `json:"account"`
}

// ActiveAccountCredential carries the decrypted runtime credential and the
// non-secret account metadata required by direct-play orchestration.
type ActiveAccountCredential struct {
	Role           models.P115AccountRole
	ProviderUserID string
	TargetParentID string
	EmbyPathPrefix string
	SourceRootID   string
	Credential     p115integration.Credential
	runtimeRef     runtimeCredentialRef
}

// SourceLocation contains only the non-sensitive source mapping metadata needed
// before DirectPlay decides whether Provider credentials are currently usable.
type SourceLocation struct {
	AccountID      string
	EmbyPathPrefix string
	SourceRootID   string
}

// SourceLocationInput updates the source account's local Emby prefix and 115 root.
type SourceLocationInput struct {
	EmbyPathPrefix string `json:"embyPathPrefix"`
	SourceRootID   string `json:"sourceRootId"`
}

// ReplaceCookieInput contains the new write-only Cookie and an optional
// client type fallback used only when the UID ssoent is unknown.
type ReplaceCookieInput struct {
	Cookie  string `json:"cookie"`
	AppType string `json:"appType"`
}

// Service owns 115 account validation rules and encrypted credential persistence.
type Service struct {
	store             accountStore
	cipher            credentialCipher
	validator         p115integration.CredentialValidator
	directoryResolver directoryResolver
	leases            p115quota.LeaseStore
	keyDeriver        *p115quota.KeyDeriver
	businessTimezone  *time.Location
	now               func() time.Time
}

// NewService builds the production account service without reading environment variables internally.
func NewService(database *gorm.DB, encryptionKey string, validator p115integration.CredentialValidator) (*Service, error) {
	return NewServiceWithLeaseStore(database, encryptionKey, validator, p115quota.UnavailableLeaseStore{}, time.UTC)
}

// NewServiceWithLeaseStore builds the account control plane with optional
// Redis-backed usage summaries. Redis failure never blocks account writes.
func NewServiceWithLeaseStore(
	database *gorm.DB,
	encryptionKey string,
	validator p115integration.CredentialValidator,
	leases p115quota.LeaseStore,
	businessTimezone *time.Location,
) (*Service, error) {
	if database == nil {
		return nil, ErrStoreUnavailable
	}
	if validator == nil {
		return nil, ErrValidatorUnavailable
	}
	box, err := secretbox.NewDerived(encryptionKey, credentialEncryptionPurpose)
	if err != nil {
		return nil, err
	}
	keyDeriver, err := p115quota.NewKeyDeriver(encryptionKey)
	if err != nil {
		return nil, err
	}
	if leases == nil {
		leases = p115quota.UnavailableLeaseStore{}
	}
	if businessTimezone == nil {
		return nil, ErrPersonalPlanPolicyUnavailable
	}
	service := &Service{
		store: &gormAccountStore{db: database}, cipher: box, validator: validator,
		leases: leases, keyDeriver: keyDeriver, businessTimezone: businessTimezone, now: time.Now,
	}
	if resolver, ok := validator.(directoryResolver); ok {
		service.directoryResolver = resolver
	}
	return service, nil
}

func newServiceWithDependencies(store accountStore, cipher credentialCipher) *Service {
	return &Service{store: store, cipher: cipher, now: time.Now}
}

// List returns all administrator-managed accounts as safe summaries.
func (s *Service) List(ctx context.Context) ([]AccountSummary, error) {
	accounts, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]AccountSummary, 0, len(accounts))
	for i := range accounts {
		items = append(items, *s.accountSummary(ctx, &accounts[i]))
	}
	return items, nil
}

// Get returns one safe account summary by identifier.
func (s *Service) Get(ctx context.Context, accountID string) (*AccountSummary, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, ErrAccountIDRequired
	}
	account, err := s.getAdminAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return s.accountSummary(ctx, account), nil
}

// Create validates role-specific fields, encrypts the Cookie, and stores a disabled pending account.
func (s *Service) Create(ctx context.Context, input CreateAccountInput) (*AccountSummary, error) {
	if err := normalizeAndValidateCreateInput(&input); err != nil {
		return nil, err
	}

	ciphertext, err := s.cipher.Encrypt(input.Cookie)
	if err != nil {
		log.Printf("[P115Account] Cookie 加密失败 role=%s errorType=%T", input.Role, err)
		return nil, err
	}

	account := &models.P115Account{
		Role:             input.Role,
		Alias:            input.Alias,
		AuthMode:         models.P115AuthModeLegacyCookie,
		CookieCiphertext: stringPointer(ciphertext),
		AppType:          stringPointer(input.AppType),
		UserAgent:        stringPointer(input.UserAgent),
		Status:           models.P115AccountStatusPending,
		Enabled:          false,
	}
	if input.Role == models.P115AccountRolePlayback {
		if input.TargetParentID != "" {
			targetParentID := input.TargetParentID
			account.TargetParentID = &targetParentID
		}
	} else {
		embyPathPrefix := input.EmbyPathPrefix
		sourceRootID := input.SourceRootID
		account.EmbyPathPrefix = &embyPathPrefix
		account.SourceRootID = &sourceRootID
	}

	if err := s.store.Create(ctx, account); err != nil {
		log.Printf("[P115Account] 创建账号失败 role=%s errorType=%T", input.Role, err)
		return nil, err
	}
	log.Printf("[P115Account] 创建账号成功 accountId=%s role=%s status=%s enabled=%t",
		account.ID, account.Role, account.Status, account.Enabled)
	return accountSummary(account), nil
}

// LoadCredentialForValidation decrypts an account for explicit validation without an activation gate.
func (s *Service) LoadCredentialForValidation(ctx context.Context, accountID string) (p115integration.Credential, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return p115integration.Credential{}, ErrAccountIDRequired
	}
	account, err := s.getAdminAccount(ctx, accountID)
	if err != nil {
		return p115integration.Credential{}, err
	}
	return s.credentialFromAccount(account, false)
}

// LoadActiveCredential decrypts a credential only when the account is enabled and active.
func (s *Service) LoadActiveCredential(ctx context.Context, accountID string) (p115integration.Credential, error) {
	return s.loadCredential(ctx, accountID, true)
}

// LoadEnabledSourceLocation returns the single manually enabled source mapping
// without reading Provider identity, decrypting Cookie data, or gating on the
// account's transient runtime health state.
func (s *Service) LoadEnabledSourceLocation(ctx context.Context) (SourceLocation, error) {
	account, err := s.store.GetEnabledSourceLocation(ctx)
	if err != nil {
		return SourceLocation{}, err
	}
	if account == nil || account.Role != models.P115AccountRoleSource || !account.Enabled ||
		strings.TrimSpace(account.ID) == "" || account.ID != strings.TrimSpace(account.ID) ||
		account.EmbyPathPrefix == nil || account.SourceRootID == nil {
		return SourceLocation{}, ErrAccountUnavailable
	}
	embyPathPrefix, sourceRootID, err := normalizeSourceLocation(*account.EmbyPathPrefix, *account.SourceRootID)
	if err != nil || embyPathPrefix != *account.EmbyPathPrefix || sourceRootID != *account.SourceRootID {
		return SourceLocation{}, ErrAccountUnavailable
	}
	return SourceLocation{
		AccountID: account.ID, EmbyPathPrefix: embyPathPrefix, SourceRootID: sourceRootID,
	}, nil
}

// LoadActiveCredentialByRole resolves the unique enabled account for one role
// and returns only the runtime metadata needed by the direct-play service.
func (s *Service) LoadActiveCredentialByRole(ctx context.Context, role models.P115AccountRole) (ActiveAccountCredential, error) {
	if role != models.P115AccountRoleSource && role != models.P115AccountRolePlayback {
		return ActiveAccountCredential{}, ErrInvalidRole
	}
	now := s.now().UTC()
	account, err := s.store.AcquireRuntimeByRole(ctx, role, now, now.Add(runtimeProviderCooldown))
	if err != nil {
		return ActiveAccountCredential{}, err
	}
	if account.ProviderUserID == nil || strings.TrimSpace(*account.ProviderUserID) == "" {
		return ActiveAccountCredential{}, ErrAccountUnavailable
	}
	targetParentID := ""
	if account.TargetParentID != nil {
		targetParentID = strings.TrimSpace(*account.TargetParentID)
	}
	embyPathPrefix := ""
	if account.EmbyPathPrefix != nil {
		embyPathPrefix = strings.TrimSpace(*account.EmbyPathPrefix)
	}
	sourceRootID := ""
	if account.SourceRootID != nil {
		sourceRootID = strings.TrimSpace(*account.SourceRootID)
	}
	if role == models.P115AccountRoleSource &&
		(targetParentID != "" || embyPathPrefix == "" || sourceRootID == "") {
		return ActiveAccountCredential{}, ErrAccountUnavailable
	}
	if role == models.P115AccountRolePlayback &&
		(targetParentID == "" || embyPathPrefix != "" || sourceRootID != "") {
		return ActiveAccountCredential{}, ErrAccountUnavailable
	}
	ciphertext, appType, userAgent, err := requiredCredentialFields(account)
	if err != nil {
		return ActiveAccountCredential{}, err
	}
	cookie, err := s.cipher.Decrypt(ciphertext)
	if err != nil {
		log.Printf("[P115Account] 运行期 Cookie 解密失败 accountId=%s role=%s errorType=%T", account.ID, role, err)
		return ActiveAccountCredential{}, err
	}
	return ActiveAccountCredential{
		Role:           role,
		ProviderUserID: strings.TrimSpace(*account.ProviderUserID),
		TargetParentID: targetParentID,
		EmbyPathPrefix: embyPathPrefix,
		SourceRootID:   sourceRootID,
		Credential: p115integration.Credential{
			AccountID: account.ID,
			Cookie:    cookie,
			AppType:   appType,
			UserAgent: userAgent,
		},
		runtimeRef: runtimeCredentialRef{
			accountID:          account.ID,
			expectedCiphertext: ciphertext,
			expectedUpdatedAt:  account.UpdatedAt,
		},
	}, nil
}

// UpdateSourceLocation changes only the source account's explicit path mapping.
func (s *Service) UpdateSourceLocation(ctx context.Context, accountID string, input SourceLocationInput) (*AccountSummary, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, ErrAccountIDRequired
	}
	if _, err := s.getAdminAccount(ctx, accountID); err != nil {
		return nil, err
	}
	embyPathPrefix, sourceRootID, err := normalizeSourceLocation(input.EmbyPathPrefix, input.SourceRootID)
	if err != nil {
		return nil, err
	}
	account, err := s.store.UpdateSourceLocation(ctx, accountID, embyPathPrefix, sourceRootID)
	if err != nil {
		return nil, err
	}
	log.Printf("[P115Account] 源目录配置更新 accountId=%s sourceRootId=%s", account.ID, sourceRootID)
	return accountSummary(account), nil
}

func (s *Service) loadCredential(ctx context.Context, accountID string, requireActive bool) (p115integration.Credential, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return p115integration.Credential{}, ErrAccountIDRequired
	}
	account, err := s.store.GetByID(ctx, accountID)
	if err != nil {
		return p115integration.Credential{}, err
	}
	return s.credentialFromAccount(account, requireActive)
}

func (s *Service) credentialFromAccount(account *models.P115Account, requireActive bool) (p115integration.Credential, error) {
	if requireActive && (!account.Enabled || account.Status != models.P115AccountStatusActive) {
		log.Printf("[P115Account] 拒绝读取非活动凭据 accountId=%s status=%s enabled=%t",
			account.ID, account.Status, account.Enabled)
		return p115integration.Credential{}, ErrAccountUnavailable
	}
	ciphertext, appType, userAgent, err := requiredCredentialFields(account)
	if err != nil {
		return p115integration.Credential{}, err
	}
	cookie, err := s.cipher.Decrypt(ciphertext)
	if err != nil {
		log.Printf("[P115Account] Cookie 解密失败 accountId=%s errorType=%T", account.ID, err)
		return p115integration.Credential{}, err
	}
	return p115integration.Credential{
		AccountID: account.ID,
		Cookie:    cookie,
		AppType:   appType,
		UserAgent: userAgent,
	}, nil
}

// ReplaceCookie encrypts a replacement Cookie, refreshes its client type, and resets validation-derived state.
func (s *Service) ReplaceCookie(ctx context.Context, accountID string, input ReplaceCookieInput) (*AccountSummary, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, ErrAccountIDRequired
	}
	var err error
	input.Cookie, err = normalizeAndValidateCookie(input.Cookie)
	if err != nil {
		return nil, err
	}
	input.AppType, err = resolveCookieAppType(input.Cookie, input.AppType)
	if err != nil {
		return nil, err
	}
	if _, err := s.getAdminAccount(ctx, accountID); err != nil {
		return nil, err
	}

	ciphertext, err := s.cipher.Encrypt(input.Cookie)
	if err != nil {
		log.Printf("[P115Account] 替换 Cookie 加密失败 accountId=%s errorType=%T", accountID, err)
		return nil, err
	}
	account, err := s.store.ReplaceCredential(ctx, accountID, credentialReplacement{
		CookieCiphertext: ciphertext,
		AppType:          input.AppType,
		Status:           models.P115AccountStatusPending,
		Enabled:          false,
	})
	if err != nil {
		log.Printf("[P115Account] 替换 Cookie 失败 accountId=%s errorType=%T", accountID, err)
		return nil, err
	}
	log.Printf("[P115Account] 替换 Cookie 成功 accountId=%s status=%s enabled=%t",
		account.ID, account.Status, account.Enabled)
	return accountSummary(account), nil
}

func normalizeAndValidateCreateInput(input *CreateAccountInput) error {
	input.Role = models.P115AccountRole(strings.TrimSpace(string(input.Role)))
	input.Alias = strings.TrimSpace(input.Alias)
	input.UserAgent = strings.TrimSpace(input.UserAgent)
	input.EmbyPathPrefix = strings.TrimSpace(input.EmbyPathPrefix)
	input.SourceRootID = strings.TrimSpace(input.SourceRootID)
	input.TargetParentID = strings.TrimSpace(input.TargetParentID)

	if input.Role != models.P115AccountRoleSource && input.Role != models.P115AccountRolePlayback {
		return ErrInvalidRole
	}
	if input.Alias == "" {
		return ErrAliasRequired
	}
	if utf8.RuneCountInString(input.Alias) > 100 {
		return ErrAliasInvalid
	}
	var err error
	input.Cookie, err = normalizeAndValidateCookie(input.Cookie)
	if err != nil {
		return err
	}
	input.AppType, err = resolveCookieAppType(input.Cookie, input.AppType)
	if err != nil {
		return err
	}
	if input.UserAgent == "" {
		return ErrUserAgentRequired
	}
	if utf8.RuneCountInString(input.UserAgent) > 512 || strings.ContainsAny(input.UserAgent, "\r\n") {
		return ErrUserAgentInvalid
	}
	if len(input.TargetParentID) > 64 {
		return ErrTargetParentInvalid
	}
	if input.Role == models.P115AccountRoleSource && input.TargetParentID != "" {
		return ErrSourceTargetParentUnexpected
	}
	if input.Role == models.P115AccountRoleSource {
		var err error
		input.EmbyPathPrefix, input.SourceRootID, err = normalizeSourceLocation(input.EmbyPathPrefix, input.SourceRootID)
		if err != nil {
			return err
		}
	} else if input.EmbyPathPrefix != "" || input.SourceRootID != "" {
		return ErrPlaybackSourceLocationUnexpected
	}
	return nil
}

// resolveCookieAppType prefers the client encoded in UID and validates the
// administrator fallback only when the provider code is unknown.
func resolveCookieAppType(cookie, fallback string) (string, error) {
	if detected, ok := p115integration.DetectCookieAppType(cookie); ok {
		return detected, nil
	}
	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		return "", ErrAppTypeRequired
	}
	if len(fallback) > 32 || !appTypePattern.MatchString(fallback) {
		return "", ErrAppTypeInvalid
	}
	return fallback, nil
}

func normalizeSourceLocation(embyPathPrefix, sourceRootID string) (string, string, error) {
	embyPathPrefix = strings.TrimSpace(embyPathPrefix)
	sourceRootID = strings.TrimSpace(sourceRootID)
	if embyPathPrefix == "" {
		return "", "", ErrEmbyPathPrefixRequired
	}
	if sourceRootID == "" {
		return "", "", ErrSourceRootIDRequired
	}
	if !validEmbyPathPrefix(embyPathPrefix) {
		return "", "", ErrEmbyPathPrefixInvalid
	}
	parsedRootID, err := strconv.ParseUint(sourceRootID, 10, 64)
	if err != nil || strconv.FormatUint(parsedRootID, 10) != sourceRootID || len(sourceRootID) > 64 {
		return "", "", ErrSourceRootIDInvalid
	}
	return embyPathPrefix, sourceRootID, nil
}

func validEmbyPathPrefix(value string) bool {
	if !utf8.ValidString(value) || len(value) > maxEmbyPathPrefixLength || value == "/" ||
		!strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") ||
		strings.ContainsAny(value, "\\\x00\r\n") {
		return false
	}
	for _, segment := range strings.Split(strings.TrimPrefix(value, "/"), "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func normalizeAndValidateCookie(cookie string) (string, error) {
	cookie = strings.TrimSpace(cookie)
	if cookie == "" {
		return "", ErrCookieRequired
	}
	if len(cookie) > maxCookieLength || strings.ContainsAny(cookie, "\r\n") {
		return "", ErrCookieInvalid
	}
	return cookie, nil
}

func accountSummary(account *models.P115Account) *AccountSummary {
	if account == nil {
		return nil
	}
	summary := &AccountSummary{
		ID:                   account.ID,
		Role:                 account.Role,
		Alias:                account.Alias,
		AuthMode:             account.AuthMode,
		ProviderUserID:       account.ProviderUserID,
		AppType:              stringValue(account.AppType),
		UserAgent:            stringValue(account.UserAgent),
		EmbyPathPrefix:       account.EmbyPathPrefix,
		SourceRootID:         account.SourceRootID,
		TargetParentID:       account.TargetParentID,
		TargetParentPath:     account.TargetParentPath,
		MaxConcurrentStreams: account.MaxConcurrentStreams,
		Status:               account.Status,
		Enabled:              account.Enabled,
		LastValidatedAt:      account.LastValidatedAt,
		LastSucceededAt:      account.LastSucceededAt,
		CooldownUntil:        account.CooldownUntil,
		LastErrorCode:        account.LastErrorCode,
		LastErrorMessage:     account.LastErrorMessage,
		CreatedAt:            account.CreatedAt,
		UpdatedAt:            account.UpdatedAt,
	}
	if account.Role == models.P115AccountRolePlayback {
		available := false
		summary.UsageAvailable = &available
		var unavailable *int
		summary.ReservedStreams = unavailable
		summary.ActiveStreams = unavailable
		summary.OccupiedStreams = unavailable
	}
	return summary
}

func requiredCredentialFields(account *models.P115Account) (string, string, string, error) {
	if account == nil || account.Status == models.P115AccountStatusRevoked ||
		account.CookieCiphertext == nil || strings.TrimSpace(*account.CookieCiphertext) == "" ||
		account.AppType == nil || strings.TrimSpace(*account.AppType) == "" ||
		account.UserAgent == nil || strings.TrimSpace(*account.UserAgent) == "" {
		return "", "", "", ErrAccountUnavailable
	}
	return *account.CookieCiphertext, strings.TrimSpace(*account.AppType), strings.TrimSpace(*account.UserAgent), nil
}

func stringPointer(value string) *string {
	return &value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *Service) getAdminAccount(ctx context.Context, accountID string) (*models.P115Account, error) {
	account, err := s.store.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account.OwnerUserID != nil || account.Status == models.P115AccountStatusRevoked {
		return nil, ErrAccountNotFound
	}
	return account, nil
}
