package p115account

import (
	"context"
	"log"
	"strings"
	"time"

	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/models"
)

const (
	personalPlaybackAlias     = "personal-playback"
	personalProviderUserAgent = "Mozilla/5.0"
)

// PersonalAccountSummary is the user-visible account view. It intentionally
// omits Cookie, Provider User-Agent, internal target ID, owner, role, and alias.
type PersonalAccountSummary struct {
	ID                            string                   `json:"id"`
	ProviderUserID                *string                  `json:"providerUserId,omitempty"`
	AppType                       string                   `json:"appType"`
	TargetParentPath              *string                  `json:"targetParentPath,omitempty"`
	MaxConcurrentStreams          *int                     `json:"maxConcurrentStreams,omitempty"`
	EffectiveMaxConcurrentStreams *int                     `json:"effectiveMaxConcurrentStreams,omitempty"`
	SimultaneousStreamLimit       *int                     `json:"simultaneousStreamLimit,omitempty"`
	PlaybackMode                  models.P115PlaybackMode  `json:"p115PlaybackMode,omitempty"`
	TransferHourlyLimit           *int                     `json:"transferHourlyLimit,omitempty"`
	TransferDailyLimit            *int                     `json:"transferDailyLimit,omitempty"`
	Status                        models.P115AccountStatus `json:"status"`
	Enabled                       bool                     `json:"enabled"`
	UsageAvailable                bool                     `json:"usageAvailable"`
	ReservedStreams               *int                     `json:"reservedStreams"`
	ActiveStreams                 *int                     `json:"activeStreams"`
	OccupiedStreams               *int                     `json:"occupiedStreams"`
	UserReservedStreams           *int                     `json:"userReservedStreams"`
	UserActiveStreams             *int                     `json:"userActiveStreams"`
	UserOccupiedStreams           *int                     `json:"userOccupiedStreams"`
	TransferPending               *int                     `json:"transferPending"`
	TransferHourlyUsed            *int                     `json:"transferHourlyUsed"`
	TransferDailyUsed             *int                     `json:"transferDailyUsed"`
	LastValidatedAt               *time.Time               `json:"lastValidatedAt,omitempty"`
	LastSucceededAt               *time.Time               `json:"lastSucceededAt,omitempty"`
	CooldownUntil                 *time.Time               `json:"cooldownUntil,omitempty"`
	LastErrorCode                 *string                  `json:"lastErrorCode,omitempty"`
	CreatedAt                     time.Time                `json:"createdAt"`
	UpdatedAt                     time.Time                `json:"updatedAt"`
}

// PersonalValidationResult reports explicit validation without exposing the credential.
type PersonalValidationResult struct {
	Valid   bool                    `json:"valid"`
	Account *PersonalAccountSummary `json:"account"`
}

// CreatePersonalAccount creates a disabled pending playback account from one
// locally inspected write-only Cookie without calling 115 or plan resolvers.
func (s *Service) CreatePersonalAccount(ctx context.Context, ownerUserID, cookie string) (*PersonalAccountSummary, error) {
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID == "" {
		return nil, ErrOwnerUserIDRequired
	}
	var err error
	cookie, err = normalizeAndValidateCookie(cookie)
	if err != nil {
		return nil, err
	}
	appType, err := p115integration.DetectPersonalCookieAppType(cookie)
	if err != nil {
		return nil, ErrCookieInvalid
	}
	ciphertext, err := s.cipher.Encrypt(cookie)
	if err != nil {
		log.Printf("[P115Account] 个人 Cookie 加密失败 ownerUserId=%s errorType=%T", ownerUserID, err)
		return nil, err
	}
	account := &models.P115Account{
		Role:             models.P115AccountRolePlayback,
		Alias:            personalPlaybackAlias,
		AuthMode:         models.P115AuthModeLegacyCookie,
		OwnerUserID:      stringPointer(ownerUserID),
		CookieCiphertext: stringPointer(ciphertext),
		AppType:          stringPointer(appType),
		UserAgent:        stringPointer(personalProviderUserAgent),
		Status:           models.P115AccountStatusPending,
		Enabled:          false,
	}
	if err := s.store.Create(ctx, account); err != nil {
		return nil, err
	}
	log.Printf("[P115Account] 个人账号已创建 accountId=%s ownerUserId=%s status=%s", account.ID, ownerUserID, account.Status)
	return personalAccountSummary(account), nil
}

// GetPersonalAccount returns only the current non-revoked account owned by the user.
func (s *Service) GetPersonalAccount(ctx context.Context, ownerUserID string) (*PersonalAccountSummary, error) {
	ownerUserID = strings.TrimSpace(ownerUserID)
	account, err := s.store.GetByOwner(ctx, ownerUserID)
	if err != nil {
		return nil, err
	}
	policy, err := s.store.GetPersonalPlanPolicy(ctx, ownerUserID)
	if err != nil {
		return nil, err
	}
	summary := personalAccountSummary(account)
	if err := applyPersonalPlanPolicy(summary, policy); err != nil {
		return nil, err
	}
	s.applyPersonalUsage(ctx, summary, account, policy)
	return summary, nil
}

// ReplacePersonalCookie resets Provider-derived fields and directory identity,
// while retaining the configured concurrency value for later plan revalidation.
func (s *Service) ReplacePersonalCookie(ctx context.Context, ownerUserID, cookie string) (*PersonalAccountSummary, error) {
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID == "" {
		return nil, ErrOwnerUserIDRequired
	}
	var err error
	cookie, err = normalizeAndValidateCookie(cookie)
	if err != nil {
		return nil, err
	}
	appType, err := p115integration.DetectPersonalCookieAppType(cookie)
	if err != nil {
		return nil, ErrCookieInvalid
	}
	ciphertext, err := s.cipher.Encrypt(cookie)
	if err != nil {
		return nil, err
	}
	account, err := s.store.ReplacePersonalCredential(ctx, ownerUserID, credentialReplacement{
		CookieCiphertext: ciphertext,
		AppType:          appType,
		Status:           models.P115AccountStatusPending,
		Enabled:          false,
	})
	if err != nil {
		return nil, err
	}
	return personalAccountSummary(account), nil
}

// ValidatePersonalAccount validates only the current user's account and keeps
// the successful result disabled until directory and concurrency are configured.
func (s *Service) ValidatePersonalAccount(ctx context.Context, ownerUserID string) (*PersonalValidationResult, error) {
	account, err := s.store.GetByOwner(ctx, strings.TrimSpace(ownerUserID))
	if err != nil {
		return nil, err
	}
	result, err := s.validateAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	return &PersonalValidationResult{Valid: result.Valid, Account: personalAccountSummaryModel(result.Account)}, nil
}

// RevokePersonalAccount atomically turns the current personal account into an
// irreversible credential-free tombstone. Redis leases are intentionally untouched.
func (s *Service) RevokePersonalAccount(ctx context.Context, ownerUserID string) error {
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID == "" {
		return ErrOwnerUserIDRequired
	}
	return s.store.RevokePersonal(ctx, ownerUserID)
}

func personalAccountSummary(account *models.P115Account) *PersonalAccountSummary {
	if account == nil {
		return nil
	}
	return &PersonalAccountSummary{
		ID:                   account.ID,
		ProviderUserID:       maskProviderUserID(account.ProviderUserID),
		AppType:              stringValue(account.AppType),
		TargetParentPath:     account.TargetParentPath,
		MaxConcurrentStreams: account.MaxConcurrentStreams,
		Status:               account.Status,
		Enabled:              account.Enabled,
		UsageAvailable:       false,
		LastValidatedAt:      account.LastValidatedAt,
		LastSucceededAt:      account.LastSucceededAt,
		CooldownUntil:        account.CooldownUntil,
		LastErrorCode:        account.LastErrorCode,
		CreatedAt:            account.CreatedAt,
		UpdatedAt:            account.UpdatedAt,
	}
}

func personalAccountSummaryModel(summary *AccountSummary) *PersonalAccountSummary {
	if summary == nil {
		return nil
	}
	return &PersonalAccountSummary{
		ID:                   summary.ID,
		ProviderUserID:       maskProviderUserID(summary.ProviderUserID),
		AppType:              summary.AppType,
		TargetParentPath:     summary.TargetParentPath,
		MaxConcurrentStreams: summary.MaxConcurrentStreams,
		Status:               summary.Status,
		Enabled:              summary.Enabled,
		UsageAvailable:       false,
		LastValidatedAt:      summary.LastValidatedAt,
		LastSucceededAt:      summary.LastSucceededAt,
		CooldownUntil:        summary.CooldownUntil,
		LastErrorCode:        summary.LastErrorCode,
		CreatedAt:            summary.CreatedAt,
		UpdatedAt:            summary.UpdatedAt,
	}
}

func maskProviderUserID(providerUserID *string) *string {
	if providerUserID == nil || strings.TrimSpace(*providerUserID) == "" {
		return nil
	}
	value := strings.TrimSpace(*providerUserID)
	masked := "****"
	if len(value) > 4 {
		masked += value[len(value)-4:]
	}
	return &masked
}
