package p115account

import (
	"context"
	"errors"
	"time"

	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var playbackRouteMetadataColumns = []string{
	"id", "role", "owner_user_id", "provider_user_id", "target_parent_id", "target_parent_path",
	"max_concurrent_streams", "status", "enabled", "cooldown_until", "updated_at",
}

// ResolvePlaybackRouteMetadata locks the user's effective plan references and
// reads the matching personal/shared account in one transaction snapshot.
func (s *gormAccountStore) ResolvePlaybackRouteMetadata(ctx context.Context, ownerUserID string) (*models.P115Account, PersonalPlanPolicy, error) {
	var account *models.P115Account
	var policy PersonalPlanPolicy
	err := s.database(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		policy, err = s.loadPersonalPlanPolicy(tx, ownerUserID, true)
		if err != nil {
			return err
		}
		var selected models.P115Account
		query := tx.Select(playbackRouteMetadataColumns)
		if policy.PlaybackMode == models.P115PlaybackModePersonal {
			query = query.Where("owner_user_id = ? AND role = ? AND status <> ? AND enabled = ?",
				ownerUserID, models.P115AccountRolePlayback, models.P115AccountStatusRevoked, true)
		} else {
			query = query.Where("owner_user_id IS NULL AND role = ? AND status <> ? AND enabled = ?",
				models.P115AccountRolePlayback, models.P115AccountStatusRevoked, true)
		}
		if err := query.First(&selected).Error; err != nil {
			return err
		}
		account = &selected
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if policy.PlaybackMode == models.P115PlaybackModePersonal {
			return nil, policy, ErrAccountNotFound
		}
		return nil, policy, ErrAccountUnavailable
	}
	if err != nil {
		return nil, policy, safeP115AccountStoreError("resolve_playback_route_metadata", err)
	}
	return account, policy, nil
}

// GetPersonalPlaybackMetadata selects no credential columns before Redis admission.
func (s *gormAccountStore) GetPersonalPlaybackMetadata(ctx context.Context, ownerUserID string) (*models.P115Account, error) {
	var account models.P115Account
	err := s.database(ctx).Select(playbackRouteMetadataColumns).
		Where("owner_user_id = ? AND role = ? AND status <> ? AND enabled = ?", ownerUserID, models.P115AccountRolePlayback, models.P115AccountStatusRevoked, true).
		First(&account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAccountNotFound
	}
	if err != nil {
		return nil, safeP115AccountStoreError("get_personal_playback_metadata", err)
	}
	return &account, nil
}

// GetSharedPlaybackMetadata selects the enabled administrator playback account
// without reading its encrypted credential.
func (s *gormAccountStore) GetSharedPlaybackMetadata(ctx context.Context) (*models.P115Account, error) {
	var account models.P115Account
	err := s.database(ctx).Select(playbackRouteMetadataColumns).
		Where("owner_user_id IS NULL AND role = ? AND status <> ? AND enabled = ?",
			models.P115AccountRolePlayback, models.P115AccountStatusRevoked, true).
		First(&account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAccountUnavailable
	}
	if err != nil {
		return nil, safeP115AccountStoreError("get_shared_playback_metadata", err)
	}
	return &account, nil
}

// AcquirePlaybackRoute loads credentials only for the exact account version
// admitted by Redis and serializes expired-cooldown half-open probes.
func (s *gormAccountStore) AcquirePlaybackRoute(ctx context.Context, route PlaybackRoute, now, probeUntil time.Time) (*models.P115Account, error) {
	var account models.P115Account
	err := s.database(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND role = ? AND enabled = ? AND updated_at = ?",
				route.AccountID, models.P115AccountRolePlayback, true, route.UpdatedAt)
		if route.OwnerUserID == "" {
			query = query.Where("owner_user_id IS NULL")
		} else {
			query = query.Where("owner_user_id = ?", route.OwnerUserID)
		}
		if err := query.First(&account).Error; err != nil {
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
				Updates(map[string]interface{}{"cooldown_until": probeUntil, "updated_at": now})
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
		return nil, ErrRuntimeStateChanged
	}
	if err != nil {
		return nil, safeP115AccountStoreError("acquire_playback_route", err)
	}
	return &account, nil
}
