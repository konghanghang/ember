package p115account

import (
	"context"
	"log"
	"strings"
	"time"

	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/security/secretbox"
	"gorm.io/gorm"
)

const credentialEncryptionPurpose = "p115-cookie"

type credentialCipher interface {
	Encrypt(plain string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

type accountStore interface {
	Create(ctx context.Context, account *models.P115Account) error
	GetByID(ctx context.Context, id string) (*models.P115Account, error)
	ReplaceCredential(ctx context.Context, id string, replacement credentialReplacement) (*models.P115Account, error)
}

type credentialReplacement struct {
	CookieCiphertext string
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
	TargetParentID string
}

// AccountSummary is the safe account view and intentionally has no credential field.
type AccountSummary struct {
	ID               string                   `json:"id"`
	Role             models.P115AccountRole   `json:"role"`
	Alias            string                   `json:"alias"`
	AuthMode         models.P115AuthMode      `json:"authMode"`
	ProviderUserID   *string                  `json:"providerUserId,omitempty"`
	AppType          string                   `json:"appType"`
	UserAgent        string                   `json:"userAgent"`
	TargetParentID   *string                  `json:"targetParentId,omitempty"`
	Status           models.P115AccountStatus `json:"status"`
	Enabled          bool                     `json:"enabled"`
	LastValidatedAt  *time.Time               `json:"lastValidatedAt,omitempty"`
	LastSucceededAt  *time.Time               `json:"lastSucceededAt,omitempty"`
	CooldownUntil    *time.Time               `json:"cooldownUntil,omitempty"`
	LastErrorCode    *string                  `json:"lastErrorCode,omitempty"`
	LastErrorMessage *string                  `json:"lastErrorMessage,omitempty"`
	CreatedAt        time.Time                `json:"createdAt"`
	UpdatedAt        time.Time                `json:"updatedAt"`
}

// Service owns 115 account validation rules and encrypted credential persistence.
type Service struct {
	store  accountStore
	cipher credentialCipher
}

// NewService builds the production account service without reading environment variables internally.
func NewService(database *gorm.DB, encryptionKey string) (*Service, error) {
	if database == nil {
		return nil, ErrStoreUnavailable
	}
	box, err := secretbox.NewDerived(encryptionKey, credentialEncryptionPurpose)
	if err != nil {
		return nil, err
	}
	return &Service{
		store:  &gormAccountStore{db: database},
		cipher: box,
	}, nil
}

func newServiceWithDependencies(store accountStore, cipher credentialCipher) *Service {
	return &Service{store: store, cipher: cipher}
}

// Create validates role-specific fields, encrypts the Cookie, and stores a disabled pending account.
func (s *Service) Create(ctx context.Context, input CreateAccountInput) (*AccountSummary, error) {
	if err := normalizeAndValidateCreateInput(&input); err != nil {
		return nil, err
	}

	ciphertext, err := s.cipher.Encrypt(input.Cookie)
	if err != nil {
		log.Printf("[P115Account] Cookie 加密失败 role=%s err=%v", input.Role, err)
		return nil, err
	}

	account := &models.P115Account{
		Role:             input.Role,
		Alias:            input.Alias,
		AuthMode:         models.P115AuthModeLegacyCookie,
		CookieCiphertext: ciphertext,
		AppType:          input.AppType,
		UserAgent:        input.UserAgent,
		Status:           models.P115AccountStatusPending,
		Enabled:          false,
	}
	if input.Role == models.P115AccountRolePlayback {
		targetParentID := input.TargetParentID
		account.TargetParentID = &targetParentID
	}

	if err := s.store.Create(ctx, account); err != nil {
		log.Printf("[P115Account] 创建账号失败 role=%s err=%v", input.Role, err)
		return nil, err
	}
	log.Printf("[P115Account] 创建账号成功 accountId=%s role=%s status=%s enabled=%t",
		account.ID, account.Role, account.Status, account.Enabled)
	return accountSummary(account), nil
}

// LoadCredentialForValidation decrypts an account for explicit validation without an activation gate.
func (s *Service) LoadCredentialForValidation(ctx context.Context, accountID string) (p115integration.Credential, error) {
	return s.loadCredential(ctx, accountID, false)
}

// LoadActiveCredential decrypts a credential only when the account is enabled and active.
func (s *Service) LoadActiveCredential(ctx context.Context, accountID string) (p115integration.Credential, error) {
	return s.loadCredential(ctx, accountID, true)
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
	if requireActive && (!account.Enabled || account.Status != models.P115AccountStatusActive) {
		log.Printf("[P115Account] 拒绝读取非活动凭据 accountId=%s status=%s enabled=%t",
			account.ID, account.Status, account.Enabled)
		return p115integration.Credential{}, ErrAccountUnavailable
	}
	cookie, err := s.cipher.Decrypt(account.CookieCiphertext)
	if err != nil {
		log.Printf("[P115Account] Cookie 解密失败 accountId=%s err=%v", account.ID, err)
		return p115integration.Credential{}, err
	}
	return p115integration.Credential{
		AccountID: account.ID,
		Cookie:    cookie,
		AppType:   account.AppType,
		UserAgent: account.UserAgent,
	}, nil
}

// ReplaceCookie encrypts a replacement Cookie and resets validation-derived state.
func (s *Service) ReplaceCookie(ctx context.Context, accountID, cookie string) (*AccountSummary, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, ErrAccountIDRequired
	}
	cookie = strings.TrimSpace(cookie)
	if cookie == "" {
		return nil, ErrCookieRequired
	}

	ciphertext, err := s.cipher.Encrypt(cookie)
	if err != nil {
		log.Printf("[P115Account] 替换 Cookie 加密失败 accountId=%s err=%v", accountID, err)
		return nil, err
	}
	account, err := s.store.ReplaceCredential(ctx, accountID, credentialReplacement{
		CookieCiphertext: ciphertext,
		Status:           models.P115AccountStatusPending,
		Enabled:          false,
	})
	if err != nil {
		log.Printf("[P115Account] 替换 Cookie 失败 accountId=%s err=%v", accountID, err)
		return nil, err
	}
	log.Printf("[P115Account] 替换 Cookie 成功 accountId=%s status=%s enabled=%t",
		account.ID, account.Status, account.Enabled)
	return accountSummary(account), nil
}

func normalizeAndValidateCreateInput(input *CreateAccountInput) error {
	input.Alias = strings.TrimSpace(input.Alias)
	input.Cookie = strings.TrimSpace(input.Cookie)
	input.AppType = strings.TrimSpace(input.AppType)
	input.UserAgent = strings.TrimSpace(input.UserAgent)
	input.TargetParentID = strings.TrimSpace(input.TargetParentID)

	if input.Role != models.P115AccountRoleSource && input.Role != models.P115AccountRolePlayback {
		return ErrInvalidRole
	}
	if input.Alias == "" {
		return ErrAliasRequired
	}
	if input.Cookie == "" {
		return ErrCookieRequired
	}
	if input.AppType == "" {
		return ErrAppTypeRequired
	}
	if input.UserAgent == "" {
		return ErrUserAgentRequired
	}
	if input.Role == models.P115AccountRolePlayback && input.TargetParentID == "" {
		return ErrPlaybackTargetParentRequired
	}
	if input.Role == models.P115AccountRoleSource && input.TargetParentID != "" {
		return ErrSourceTargetParentUnexpected
	}
	return nil
}

func accountSummary(account *models.P115Account) *AccountSummary {
	if account == nil {
		return nil
	}
	return &AccountSummary{
		ID:               account.ID,
		Role:             account.Role,
		Alias:            account.Alias,
		AuthMode:         account.AuthMode,
		ProviderUserID:   account.ProviderUserID,
		AppType:          account.AppType,
		UserAgent:        account.UserAgent,
		TargetParentID:   account.TargetParentID,
		Status:           account.Status,
		Enabled:          account.Enabled,
		LastValidatedAt:  account.LastValidatedAt,
		LastSucceededAt:  account.LastSucceededAt,
		CooldownUntil:    account.CooldownUntil,
		LastErrorCode:    account.LastErrorCode,
		LastErrorMessage: account.LastErrorMessage,
		CreatedAt:        account.CreatedAt,
		UpdatedAt:        account.UpdatedAt,
	}
}
