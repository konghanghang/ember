package p115account

import (
	"context"
	"errors"

	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

type gormAccountStore struct {
	db *gorm.DB
}

func (s *gormAccountStore) Create(ctx context.Context, account *models.P115Account) error {
	return s.db.WithContext(ctx).Create(account).Error
}

func (s *gormAccountStore) GetByID(ctx context.Context, id string) (*models.P115Account, error) {
	var account models.P115Account
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAccountNotFound
	}
	if err != nil {
		return nil, err
	}
	return &account, nil
}

func (s *gormAccountStore) ReplaceCredential(ctx context.Context, id string, replacement credentialReplacement) (*models.P115Account, error) {
	var account models.P115Account
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
		return nil, err
	}
	return &account, nil
}
