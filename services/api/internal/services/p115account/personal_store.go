package p115account

import (
	"context"
	"errors"
	"time"

	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GetPersonalPlanPolicy loads the user's current explicit or default plan and
// its persisted Emby policy template without inventing fallback values.
func (s *gormAccountStore) GetPersonalPlanPolicy(ctx context.Context, ownerUserID string) (PersonalPlanPolicy, error) {
	policy, err := s.loadPersonalPlanPolicy(s.database(ctx), ownerUserID, false)
	if err != nil {
		return PersonalPlanPolicy{}, safeP115AccountStoreError("get_personal_plan_policy", err)
	}
	return policy, nil
}

// UpdatePersonalDirectory conditionally stores a resolved path/ID pair for the
// exact owned credential generation that performed the Provider lookup.
func (s *gormAccountStore) UpdatePersonalDirectory(
	ctx context.Context,
	ownerUserID, expectedCiphertext string,
	expectedUpdatedAt time.Time,
	targetParentPath, targetParentID string,
) (*models.P115Account, error) {
	result := s.database(ctx).Model(&models.P115Account{}).
		Where("owner_user_id = ? AND role = ? AND status = ? AND cookie_ciphertext = ? AND updated_at = ?",
			ownerUserID, models.P115AccountRolePlayback, models.P115AccountStatusActive, expectedCiphertext, expectedUpdatedAt).
		Updates(map[string]interface{}{
			"target_parent_path": targetParentPath,
			"target_parent_id":   targetParentID,
			"updated_at":         gorm.Expr("CURRENT_TIMESTAMP"),
		})
	if result.Error != nil {
		return nil, safeP115AccountStoreError("update_personal_directory", result.Error)
	}
	if result.RowsAffected == 0 {
		if _, err := s.GetByOwner(ctx, ownerUserID); err != nil {
			return nil, err
		}
		return nil, ErrRuntimeStateChanged
	}
	return s.GetByOwner(ctx, ownerUserID)
}

// UpdatePersonalConcurrency locks the owned account and its current plan
// policy, validates the limit, and writes the account in one transaction.
func (s *gormAccountStore) UpdatePersonalConcurrency(ctx context.Context, ownerUserID string, maxConcurrentStreams int) (*models.P115Account, PersonalPlanPolicy, error) {
	var account models.P115Account
	var policy PersonalPlanPolicy
	err := s.database(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("owner_user_id = ? AND role = ? AND status <> ?", ownerUserID, models.P115AccountRolePlayback, models.P115AccountStatusRevoked).
			First(&account).Error; err != nil {
			return err
		}
		var err error
		policy, err = s.loadPersonalPlanPolicy(tx, ownerUserID, true)
		if err != nil {
			return err
		}
		if _, err := effectivePersonalConcurrentLimit(maxConcurrentStreams, policy.SimultaneousStreamLimit); err != nil {
			return err
		}
		if err := tx.Model(&models.P115Account{}).Where("id = ?", account.ID).Updates(map[string]interface{}{
			"max_concurrent_streams": maxConcurrentStreams,
			"updated_at":             gorm.Expr("CURRENT_TIMESTAMP"),
		}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", account.ID).First(&account).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, PersonalPlanPolicy{}, ErrAccountNotFound
	}
	if err != nil {
		return nil, PersonalPlanPolicy{}, safeP115AccountStoreError("update_personal_concurrency", err)
	}
	return &account, policy, nil
}

// SetPersonalEnabled locks the account and current plan policy together before
// enabling. Disabling deliberately skips plan lookup so it cannot be blocked by
// a temporarily unavailable template.
func (s *gormAccountStore) SetPersonalEnabled(ctx context.Context, ownerUserID string, enabled bool) (*models.P115Account, PersonalPlanPolicy, error) {
	var account models.P115Account
	var policy PersonalPlanPolicy
	err := s.database(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("owner_user_id = ? AND role = ? AND status <> ?", ownerUserID, models.P115AccountRolePlayback, models.P115AccountStatusRevoked).
			First(&account).Error; err != nil {
			return err
		}
		if enabled {
			var err error
			policy, err = s.loadPersonalPlanPolicy(tx, ownerUserID, true)
			if err != nil {
				return err
			}
			if err := validateP115AccountEnableState(&account); err != nil {
				return err
			}
			if account.MaxConcurrentStreams == nil {
				return ErrAccountUnavailable
			}
			if _, err := effectivePersonalConcurrentLimit(*account.MaxConcurrentStreams, policy.SimultaneousStreamLimit); err != nil {
				return err
			}
		}
		if account.Enabled == enabled {
			return nil
		}
		if err := tx.Model(&models.P115Account{}).Where("id = ?", account.ID).Updates(map[string]interface{}{
			"enabled":    enabled,
			"updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
		}).Error; err != nil {
			return mapP115AccountConstraintError(err)
		}
		account.Enabled = enabled
		return tx.Where("id = ?", account.ID).First(&account).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, PersonalPlanPolicy{}, ErrAccountNotFound
	}
	if err != nil {
		return nil, PersonalPlanPolicy{}, safeP115AccountStoreError("set_personal_enabled", err)
	}
	return &account, policy, nil
}

func (s *gormAccountStore) loadPersonalPlanPolicy(tx *gorm.DB, ownerUserID string, lock bool) (PersonalPlanPolicy, error) {
	query := tx
	if lock {
		query = query.Clauses(clause.Locking{Strength: "SHARE"})
	}
	var user models.User
	if err := query.Select("id", "role", "plan_group").Where("id = ? AND role = ?", ownerUserID, "user").First(&user).Error; err != nil {
		return PersonalPlanPolicy{}, ErrPersonalPlanPolicyUnavailable
	}

	query = tx
	if lock {
		query = query.Clauses(clause.Locking{Strength: "SHARE"})
	}
	var group models.PlanGroup
	if user.PlanGroup != nil {
		if err := query.Where("key = ?", *user.PlanGroup).First(&group).Error; err != nil {
			return PersonalPlanPolicy{}, ErrPersonalPlanPolicyUnavailable
		}
	} else if err := query.Where("is_default = ?", true).Order("key ASC").First(&group).Error; err != nil {
		return PersonalPlanPolicy{}, ErrPersonalPlanPolicyUnavailable
	}

	query = tx
	if lock {
		query = query.Clauses(clause.Locking{Strength: "SHARE"})
	}
	var template models.PlanGroupEmbyPolicyTemplate
	if err := query.Where("plan_group_key = ?", group.Key).First(&template).Error; err != nil {
		return PersonalPlanPolicy{}, ErrPersonalPlanPolicyUnavailable
	}
	policy := PersonalPlanPolicy{
		PlanGroupKey:            group.Key,
		PlaybackMode:            group.P115PlaybackMode,
		TransferHourlyLimit:     group.P115TransferHourlyLimit,
		TransferDailyLimit:      group.P115TransferDailyLimit,
		SimultaneousStreamLimit: template.SimultaneousStreamLimit,
	}
	if err := validatePersonalPlanPolicy(policy); err != nil {
		return PersonalPlanPolicy{}, err
	}
	return policy, nil
}

func validatePersonalPlanPolicy(policy PersonalPlanPolicy) error {
	if policy.PlanGroupKey == "" ||
		(policy.PlaybackMode != models.P115PlaybackModePersonal && policy.PlaybackMode != models.P115PlaybackModeSystem) ||
		policy.TransferHourlyLimit < 1 || policy.TransferHourlyLimit > 100 ||
		policy.TransferDailyLimit < 1 || policy.TransferDailyLimit > 1000 ||
		policy.SimultaneousStreamLimit < 0 || policy.SimultaneousStreamLimit > 100 {
		return ErrPersonalPlanPolicyUnavailable
	}
	return nil
}
