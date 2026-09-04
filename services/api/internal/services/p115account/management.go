package p115account

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/models"
)

const (
	validationCodeRejected    = "credential_rejected"
	validationCodeUnavailable = "provider_unavailable"
	validationCodeProtocol    = "provider_protocol_error"
)

// Validate checks the current Cookie and persists a state transition tied to that exact ciphertext.
func (s *Service) Validate(ctx context.Context, accountID string) (*ValidationResult, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, ErrAccountIDRequired
	}
	if s.validator == nil {
		return nil, ErrValidatorUnavailable
	}
	account, err := s.getAdminAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return s.validateAccount(ctx, account)
}

func (s *Service) validateAccount(ctx context.Context, account *models.P115Account) (*ValidationResult, error) {
	ciphertext, appType, userAgent, err := requiredCredentialFields(account)
	if err != nil {
		return nil, err
	}
	cookie, err := s.cipher.Decrypt(ciphertext)
	if err != nil {
		log.Printf("[P115Account] 验证前 Cookie 解密失败 accountId=%s errorType=%T", account.ID, err)
		return nil, err
	}

	identity, validationErr := s.validator.ValidateCredential(ctx, p115integration.Credential{
		AccountID: account.ID,
		Cookie:    cookie,
		AppType:   appType,
		UserAgent: userAgent,
	})
	validatedAt := s.now().UTC()
	if errors.Is(validationErr, p115integration.ErrCredentialRejected) {
		updated, updateErr := s.store.CompleteValidationRejected(ctx, account.ID, ciphertext, validatedAt)
		if updateErr != nil {
			return nil, updateErr
		}
		log.Printf("[P115Account] Cookie 验证失效 accountId=%s status=%s", updated.ID, updated.Status)
		return &ValidationResult{Valid: false, Account: accountSummary(updated)}, nil
	}
	if validationErr != nil {
		return s.completeValidationError(ctx, account, validationErr, validatedAt)
	}

	providerUserID := strings.TrimSpace(identity.ProviderUserID)
	if providerUserID == "" {
		return s.completeValidationError(ctx, account, p115integration.ErrProviderProtocol, validatedAt)
	}
	updated, err := s.store.CompleteValidationSuccess(ctx, account.ID, ciphertext, providerUserID, validatedAt)
	if err != nil {
		return nil, err
	}
	log.Printf("[P115Account] Cookie 验证成功 accountId=%s role=%s status=%s enabled=%t",
		updated.ID, updated.Role, updated.Status, updated.Enabled)
	return &ValidationResult{Valid: true, Account: accountSummary(updated)}, nil
}

func (s *Service) completeValidationError(ctx context.Context, account *models.P115Account, validationErr error, validatedAt time.Time) (*ValidationResult, error) {
	code := validationCodeUnavailable
	message := "115 服务暂不可用"
	publicErr := p115integration.ErrProviderUnavailable
	if errors.Is(validationErr, p115integration.ErrProviderProtocol) {
		code = validationCodeProtocol
		message = "115 响应格式不兼容"
		publicErr = p115integration.ErrProviderProtocol
	}
	if account.CookieCiphertext == nil {
		return nil, ErrAccountUnavailable
	}
	if _, err := s.store.CompleteValidationError(ctx, account.ID, *account.CookieCiphertext, code, message, validatedAt); err != nil {
		return nil, err
	}
	log.Printf("[P115Account] Cookie 验证失败 accountId=%s code=%s errorType=%T", account.ID, code, validationErr)
	return nil, publicErr
}

// SetEnabled atomically enables only a validated active account or disables any existing account.
func (s *Service) SetEnabled(ctx context.Context, accountID string, enabled bool) (*AccountSummary, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, ErrAccountIDRequired
	}
	if _, err := s.getAdminAccount(ctx, accountID); err != nil {
		return nil, err
	}
	account, err := s.store.SetEnabled(ctx, accountID, enabled)
	if err != nil {
		return nil, err
	}
	log.Printf("[P115Account] 账号启用状态更新 accountId=%s role=%s enabled=%t status=%s",
		account.ID, account.Role, account.Enabled, account.Status)
	return accountSummary(account), nil
}
