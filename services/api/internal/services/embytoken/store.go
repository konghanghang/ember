package embytoken

import (
	"bytes"
	"context"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormlogger "gorm.io/gorm/logger"
)

type gormMappingStore struct {
	db *gorm.DB
}

// FindUserByEmbyID resolves the unique local binding for an Emby user ID.
func (store *gormMappingStore) FindUserByEmbyID(ctx context.Context, embyUserID string) (*models.User, error) {
	var user models.User
	err := store.database(ctx).Where("emby_id = ?", embyUserID).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, safeMappingStoreError("find_user_by_emby_id", err)
	}
	return &user, nil
}

// FindUserByID reloads current authorization state for one mapped user.
func (store *gormMappingStore) FindUserByID(ctx context.Context, userID string) (*models.User, error) {
	var user models.User
	err := store.database(ctx).Where("id = ?", userID).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, safeMappingStoreError("find_user_by_id", err)
	}
	return &user, nil
}

// UpsertMapping inserts a new digest or reactivates the same identity after a
// successful authentication. An active digest can never move to another user.
func (store *gormMappingStore) UpsertMapping(ctx context.Context, input upsertMappingInput) (*models.EmbyAccessToken, error) {
	var mapping models.EmbyAccessToken
	err := store.database(ctx).Transaction(func(tx *gorm.DB) error {
		userID := input.UserID
		candidate := models.EmbyAccessToken{
			ServerID: input.ServerID, TokenHash: append([]byte(nil), input.TokenHash...),
			EmbyUserID: input.EmbyUserID, UserID: &userID, DeviceID: input.DeviceID,
			ClientName: input.ClientName, LastSeenAt: input.At,
		}
		result := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "server_id"}, {Name: "token_hash"}},
			DoNothing: true,
		}).Create(&candidate)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			mapping = candidate
			return nil
		}

		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("server_id = ? AND token_hash = ?", input.ServerID, input.TokenHash).
			First(&mapping).Error; err != nil {
			return err
		}
		if mapping.EmbyUserID != input.EmbyUserID {
			return ErrTokenIdentityConflict
		}
		if mapping.RevokedAt == nil && (mapping.UserID == nil || *mapping.UserID != input.UserID) {
			return ErrTokenIdentityConflict
		}
		if err := tx.Model(&models.EmbyAccessToken{}).Where("id = ?", mapping.ID).Updates(map[string]interface{}{
			"user_id":        input.UserID,
			"device_id":      input.DeviceID,
			"client_name":    input.ClientName,
			"last_seen_at":   input.At,
			"revoked_at":     nil,
			"revoked_reason": nil,
			"revoked_by":     nil,
			"updated_at":     input.At,
		}).Error; err != nil {
			return err
		}
		mappingID := mapping.ID
		mapping = models.EmbyAccessToken{}
		return tx.Where("id = ?", mappingID).First(&mapping).Error
	})
	if err != nil {
		return nil, safeMappingStoreError("upsert_mapping", err)
	}
	return &mapping, nil
}

// FindMapping loads both active and revoked rows so the Service can preserve
// an explicit revoked error instead of treating revocation as absence.
func (store *gormMappingStore) FindMapping(ctx context.Context, serverID string, tokenHash []byte) (*models.EmbyAccessToken, error) {
	var mapping models.EmbyAccessToken
	err := store.database(ctx).Where("server_id = ? AND token_hash = ?", serverID, tokenHash).First(&mapping).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTokenNotFound
	}
	if err != nil {
		return nil, safeMappingStoreError("find_mapping", err)
	}
	if !bytes.Equal(mapping.TokenHash, tokenHash) {
		return nil, ErrTokenNotFound
	}
	return &mapping, nil
}

// TouchLastSeen applies a conditional timestamp update and treats a recent or
// concurrently updated row as a successful no-op.
func (store *gormMappingStore) TouchLastSeen(ctx context.Context, mappingID string, at, cutoff time.Time) error {
	result := store.database(ctx).Model(&models.EmbyAccessToken{}).
		Where("id = ? AND revoked_at IS NULL AND last_seen_at < ?", mappingID, cutoff).
		Updates(map[string]interface{}{"last_seen_at": at, "updated_at": at})
	if result.Error != nil {
		return safeMappingStoreError("touch_last_seen", result.Error)
	}
	return nil
}

// RevokeToken soft-revokes one mapping on the configured Server.
func (store *gormMappingStore) RevokeToken(ctx context.Context, input revokeInput) (int64, error) {
	result := store.revoke(ctx, store.database(ctx).Model(&models.EmbyAccessToken{}).
		Where("id = ? AND server_id = ? AND revoked_at IS NULL", input.MappingID, input.ServerID), input)
	return result.RowsAffected, result.Error
}

// RevokeDevice soft-revokes all active mappings for one device and user.
func (store *gormMappingStore) RevokeDevice(ctx context.Context, input revokeInput) (int64, error) {
	result := store.revoke(ctx, store.database(ctx).Model(&models.EmbyAccessToken{}).
		Where("server_id = ? AND user_id = ? AND device_id = ? AND revoked_at IS NULL",
			input.ServerID, input.UserID, input.DeviceID), input)
	return result.RowsAffected, result.Error
}

// RevokeDeviceAcrossServers soft-revokes one user's device mappings without a
// runtime ServerId dependency. The phase-one control plane has one active Emby
// Server and conservatively closes historical mappings as well.
func (store *gormMappingStore) RevokeDeviceAcrossServers(ctx context.Context, input revokeInput) (int64, error) {
	result := store.revoke(ctx, store.database(ctx).Model(&models.EmbyAccessToken{}).
		Where("user_id = ? AND device_id = ? AND revoked_at IS NULL", input.UserID, input.DeviceID), input)
	return result.RowsAffected, result.Error
}

// RevokeUser soft-revokes all active mappings for one Ember user across Servers.
func (store *gormMappingStore) RevokeUser(ctx context.Context, input revokeInput) (int64, error) {
	result := store.revoke(ctx, store.database(ctx).Model(&models.EmbyAccessToken{}).
		Where("user_id = ? AND revoked_at IS NULL", input.UserID), input)
	return result.RowsAffected, result.Error
}

// revoke applies the shared soft-revocation audit fields to an already scoped
// active-row update.
func (store *gormMappingStore) revoke(_ context.Context, query *gorm.DB, input revokeInput) *gorm.DB {
	result := query.Updates(map[string]interface{}{
		"revoked_at": input.At, "revoked_reason": string(input.Reason),
		"revoked_by": input.RevokedBy, "updated_at": input.At,
	})
	if result.Error != nil {
		result.Error = safeMappingStoreError("revoke", result.Error)
	}
	return result
}

// database returns a request-scoped silent GORM session so database failures
// cannot print raw query arguments such as Token digests.
func (store *gormMappingStore) database(ctx context.Context) *gorm.DB {
	return store.db.Session(&gorm.Session{Logger: gormlogger.Default.LogMode(gormlogger.Silent)}).WithContext(ctx)
}

// safeMappingStoreError preserves domain errors and reduces storage failures
// to bounded diagnostics without SQL values.
func safeMappingStoreError(operation string, err error) error {
	if err == nil || errors.Is(err, ErrUserNotFound) || errors.Is(err, ErrTokenNotFound) ||
		errors.Is(err, ErrTokenIdentityConflict) || errors.Is(err, ErrStoreUnavailable) {
		return err
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		log.Printf("[EmbyTokenStore] 数据库操作失败 operation=%s code=%s constraint=%s",
			operation, pgErr.Code, pgErr.ConstraintName)
	} else {
		log.Printf("[EmbyTokenStore] 存储操作失败 operation=%s errorType=%T", operation, err)
	}
	return ErrStoreUnavailable
}
