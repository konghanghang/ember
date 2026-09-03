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

// GetEnabledSourceLocation reads only the non-sensitive columns required for
// local path mapping. Runtime status and Provider credential state are
// intentionally excluded from this query.
func (s *gormAccountStore) GetEnabledSourceLocation(ctx context.Context) (*models.P115Account, error) {
	var account models.P115Account
	err := s.database(ctx).
		Select("id", "role", "enabled", "emby_path_prefix", "source_root_id").
		Where("role = ? AND enabled = ?", models.P115AccountRoleSource, true).
		First(&account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAccountUnavailable
	}
	if err != nil {
		return nil, safeP115AccountStoreError("get_enabled_source_location", err)
	}
	return &account, nil
}

// AcquireRuntimeByRole returns an active account or leases one expired
// cooldown probe while holding the account row lock. The probe lease keeps
// concurrent Gateway replicas from retrying the same Provider account.
func (s *gormAccountStore) AcquireRuntimeByRole(
	ctx context.Context,
	role models.P115AccountRole,
	now time.Time,
	probeUntil time.Time,
) (*models.P115Account, error) {
	var account models.P115Account
	err := s.database(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("role = ? AND enabled = ?", role, true).
			First(&account).Error; err != nil {
			return err
		}
		switch account.Status {
		case models.P115AccountStatusActive:
			return nil
		case models.P115AccountStatusCoolingDown:
			if account.CooldownUntil == nil || account.CooldownUntil.After(now) {
				return ErrAccountCoolingDown
			}
			result := tx.Model(&models.P115Account{}).
				Where("id = ? AND status = ? AND updated_at = ?", account.ID, models.P115AccountStatusCoolingDown, account.UpdatedAt).
				Updates(map[string]interface{}{
					"cooldown_until": probeUntil,
					"updated_at":     now,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return ErrRuntimeStateChanged
			}
			return tx.Where("id = ?", account.ID).First(&account).Error
		default:
			return ErrAccountUnavailable
		}
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAccountUnavailable
	}
	if err != nil {
		return nil, safeP115AccountStoreError("acquire_runtime_by_role", err)
	}
	return &account, nil
}

// UpdateSourceLocation changes only a source account while holding its row lock.
func (s *gormAccountStore) UpdateSourceLocation(ctx context.Context, id, embyPathPrefix, sourceRootID string) (*models.P115Account, error) {
	var account models.P115Account
	err := s.database(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(&account).Error; err != nil {
			return err
		}
		if account.Role != models.P115AccountRoleSource {
			return ErrSourceLocationOnly
		}
		if err := tx.Model(&models.P115Account{}).Where("id = ?", id).Updates(map[string]interface{}{
			"emby_path_prefix": embyPathPrefix,
			"source_root_id":   sourceRootID,
			"updated_at":       gorm.Expr("CURRENT_TIMESTAMP"),
		}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).First(&account).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAccountNotFound
	}
	if err != nil {
		return nil, safeP115AccountStoreError("update_source_location", err)
	}
	return &account, nil
}

func (s *gormAccountStore) ReplaceCredential(ctx context.Context, id string, replacement credentialReplacement) (*models.P115Account, error) {
	var account models.P115Account
	err := s.database(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.P115Account{}).Where("id = ?", id).Updates(map[string]interface{}{
			"cookie_ciphertext":  replacement.CookieCiphertext,
			"app_type":           replacement.AppType,
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

// CompleteRuntimeHealth applies a fixed state transition only to the exact
// credential and account generation used by the finished playback request.
func (s *gormAccountStore) CompleteRuntimeHealth(
	ctx context.Context,
	ref runtimeCredentialRef,
	mutation runtimeHealthMutation,
) error {
	updates := map[string]interface{}{
		"status":             mutation.Status,
		"cooldown_until":     mutation.CooldownUntil,
		"last_error_code":    nil,
		"last_error_message": nil,
		"updated_at":         mutation.At,
	}
	if mutation.Disable {
		updates["enabled"] = false
	}
	if mutation.Succeeded {
		updates["last_succeeded_at"] = mutation.At
	}
	if mutation.Code != "" {
		updates["last_error_code"] = mutation.Code
		updates["last_error_message"] = mutation.Message
	}

	result := s.database(ctx).Model(&models.P115Account{}).
		Where("id = ? AND cookie_ciphertext = ? AND updated_at = ?", ref.accountID, ref.expectedCiphertext, ref.expectedUpdatedAt).
		Updates(updates)
	if result.Error != nil {
		return safeP115AccountStoreError("complete_runtime_health", result.Error)
	}
	if result.RowsAffected == 0 {
		var count int64
		if err := s.database(ctx).Model(&models.P115Account{}).Where("id = ?", ref.accountID).Count(&count).Error; err != nil {
			return safeP115AccountStoreError("complete_runtime_health", err)
		}
		if count == 0 {
			return ErrAccountNotFound
		}
		return ErrRuntimeStateChanged
	}
	return nil
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
	if account.Role == models.P115AccountRoleSource &&
		(account.TargetParentID != nil || account.EmbyPathPrefix == nil || strings.TrimSpace(*account.EmbyPathPrefix) == "" ||
			account.SourceRootID == nil || strings.TrimSpace(*account.SourceRootID) == "") {
		return ErrAccountUnavailable
	}
	if account.Role == models.P115AccountRolePlayback &&
		(account.TargetParentID == nil || strings.TrimSpace(*account.TargetParentID) == "" ||
			account.EmbyPathPrefix != nil || account.SourceRootID != nil) {
		return ErrAccountUnavailable
	}
	return nil
}

func safeP115AccountStoreError(operation string, err error) error {
	if err == nil || errors.Is(err, ErrAccountNotFound) || errors.Is(err, ErrCredentialChanged) ||
		errors.Is(err, ErrAccountCoolingDown) || errors.Is(err, ErrRuntimeStateChanged) ||
		errors.Is(err, ErrAccountUnavailable) || errors.Is(err, ErrRoleAlreadyEnabled) ||
		errors.Is(err, ErrProviderUserAlreadyEnabled) || errors.Is(err, ErrSourceLocationOnly) {
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
