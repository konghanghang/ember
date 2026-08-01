package p115account

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormlogger "gorm.io/gorm/logger"
)

var errStoreOperation = errors.New("115 账号存储操作失败")

const (
	enabledRoleConstraint         = "uq_p115_accounts_enabled_role"
	enabledProviderUserConstraint = "uq_p115_accounts_enabled_provider_user"
)

type gormAccountStore struct {
	db *gorm.DB
}

func (s *gormAccountStore) Create(ctx context.Context, account *models.P115Account) error {
	return safeP115AccountStoreError("create", s.database(ctx).Create(account).Error)
}

func (s *gormAccountStore) List(ctx context.Context) ([]models.P115Account, error) {
	var accounts []models.P115Account
	err := s.database(ctx).
		Order("role ASC").
		Order("created_at ASC").
		Order("id ASC").
		Find(&accounts).Error
	return accounts, safeP115AccountStoreError("list", err)
}

func (s *gormAccountStore) GetByID(ctx context.Context, id string) (*models.P115Account, error) {
	var account models.P115Account
	err := s.database(ctx).Where("id = ?", id).First(&account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAccountNotFound
	}
	if err != nil {
		return nil, safeP115AccountStoreError("get", err)
	}
	return &account, nil
}

func (s *gormAccountStore) ReplaceCredential(ctx context.Context, id string, replacement credentialReplacement) (*models.P115Account, error) {
	var account models.P115Account
	err := s.database(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.P115Account{}).Where("id = ?", id).Updates(map[string]interface{}{
			"cookie_ciphertext":  replacement.CookieCiphertext,
			"provider_user_id":   nil,
			"status":             replacement.Status,
			"enabled":            replacement.Enabled,
			"last_validated_at":  nil,
			"last_succeeded_at":  nil,
			"cooldown_until":     nil,
			"last_error_code":    nil,
			"last_error_message": nil,
			"updated_at":         gorm.Expr("CURRENT_TIMESTAMP"),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrAccountNotFound
		}
		return tx.Where("id = ?", id).First(&account).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAccountNotFound
	}
	if err != nil {
		return nil, safeP115AccountStoreError("replace_credential", err)
	}
	return &account, nil
}

func (s *gormAccountStore) CompleteValidationSuccess(
	ctx context.Context,
	id, expectedCiphertext, providerUserID string,
	at time.Time,
) (*models.P115Account, error) {
	return s.completeValidation(ctx, id, expectedCiphertext, map[string]interface{}{
		"provider_user_id":   providerUserID,
		"status":             models.P115AccountStatusActive,
		"last_validated_at":  at,
		"last_succeeded_at":  at,
		"cooldown_until":     nil,
		"last_error_code":    nil,
		"last_error_message": nil,
		"updated_at":         at,
	})
}

func (s *gormAccountStore) CompleteValidationRejected(
	ctx context.Context,
	id, expectedCiphertext string,
	at time.Time,
) (*models.P115Account, error) {
	code := validationCodeRejected
	message := "115 Cookie 已失效"
	return s.completeValidation(ctx, id, expectedCiphertext, map[string]interface{}{
		"status":             models.P115AccountStatusExpired,
		"enabled":            false,
		"last_validated_at":  at,
		"cooldown_until":     nil,
		"last_error_code":    code,
		"last_error_message": message,
		"updated_at":         at,
	})
}

func (s *gormAccountStore) CompleteValidationError(
	ctx context.Context,
	id, expectedCiphertext, code, message string,
	at time.Time,
) (*models.P115Account, error) {
	return s.completeValidation(ctx, id, expectedCiphertext, map[string]interface{}{
		"status":             models.P115AccountStatusError,
		"last_validated_at":  at,
		"last_error_code":    code,
		"last_error_message": message,
		"updated_at":         at,
	})
}

func (s *gormAccountStore) completeValidation(
	ctx context.Context,
	id, expectedCiphertext string,
	updates map[string]interface{},
) (*models.P115Account, error) {
	var account models.P115Account
	err := s.database(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.P115Account{}).
			Where("id = ? AND cookie_ciphertext = ?", id, expectedCiphertext).
			Updates(updates)
		if result.Error != nil {
			return mapP115AccountConstraintError(result.Error)
		}
		if result.RowsAffected == 0 {
			var count int64
			if err := tx.Model(&models.P115Account{}).Where("id = ?", id).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return ErrAccountNotFound
			}
			return ErrCredentialChanged
		}
		return tx.Where("id = ?", id).First(&account).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAccountNotFound
	}
	if err != nil {
		return nil, safeP115AccountStoreError("complete_validation", err)
	}
	return &account, nil
}

func (s *gormAccountStore) SetEnabled(ctx context.Context, id string, enabled bool) (*models.P115Account, error) {
	var account models.P115Account
	err := s.database(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(&account).Error; err != nil {
			return err
		}
		if enabled {
			if err := validateP115AccountEnableState(&account); err != nil {
				return err
			}
		}
		if account.Enabled == enabled {
			return nil
		}
		if err := tx.Model(&models.P115Account{}).
			Where("id = ?", id).
			Updates(map[string]interface{}{
				"enabled":    enabled,
				"updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
			}).Error; err != nil {
			return mapP115AccountConstraintError(err)
		}
		account.Enabled = enabled
		return tx.Where("id = ?", id).First(&account).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAccountNotFound
	}
	if err != nil {
		return nil, safeP115AccountStoreError("set_enabled", err)
	}
	return &account, nil
}

func (s *gormAccountStore) database(ctx context.Context) *gorm.DB {
	// PostgreSQL error Detail may contain the full failed row, including Cookie ciphertext.
	// This store emits its own redacted diagnostics after each operation instead.
	return s.db.Session(&gorm.Session{Logger: gormlogger.Default.LogMode(gormlogger.Silent)}).WithContext(ctx)
}

func validateP115AccountEnableState(account *models.P115Account) error {
	if account == nil || account.Status != models.P115AccountStatusActive ||
		account.ProviderUserID == nil || strings.TrimSpace(*account.ProviderUserID) == "" ||
		account.LastValidatedAt == nil {
		return ErrAccountUnavailable
	}
	return nil
}

func safeP115AccountStoreError(operation string, err error) error {
	if err == nil || errors.Is(err, ErrAccountNotFound) || errors.Is(err, ErrCredentialChanged) ||
		errors.Is(err, ErrAccountUnavailable) || errors.Is(err, ErrRoleAlreadyEnabled) ||
		errors.Is(err, ErrProviderUserAlreadyEnabled) {
		return err
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		log.Printf("[P115AccountStore] 数据库操作失败 operation=%s code=%s constraint=%s",
			operation, pgErr.Code, pgErr.ConstraintName)
	} else {
		log.Printf("[P115AccountStore] 存储操作失败 operation=%s errorType=%T", operation, err)
	}
	return fmt.Errorf("%w: %s", errStoreOperation, operation)
}

func mapP115AccountConstraintError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return err
	}
	switch strings.ToLower(pgErr.ConstraintName) {
	case enabledRoleConstraint:
		return ErrRoleAlreadyEnabled
	case enabledProviderUserConstraint:
		return ErrProviderUserAlreadyEnabled
	default:
		return err
	}
}
