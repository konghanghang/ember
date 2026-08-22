package directplay

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var activeTransferStatuses = []models.PlaybackTransferTaskStatus{
	models.PlaybackTransferTaskStatusPending,
	models.PlaybackTransferTaskStatusInitializing,
	models.PlaybackTransferTaskStatusChallenging,
	models.PlaybackTransferTaskStatusVerifying,
}

type gormTaskStore struct {
	db *gorm.DB
}

// BeginAttempt closes any orphaned active record after the caller owns the
// content advisory lock, then creates one new pending attempt.
func (store *gormTaskStore) BeginAttempt(ctx context.Context, input beginAttemptInput) (*models.PlaybackTransferTask, error) {
	var task models.PlaybackTransferTask
	err := store.database(ctx).Transaction(func(tx *gorm.DB) error {
		interruptedCode := "interrupted"
		interruptedMessage := "previous transfer interrupted"
		if err := tx.Model(&models.PlaybackTransferTask{}).
			Where("playback_account_id = ? AND sha1 = ? AND size = ? AND status IN ?",
				input.PlaybackAccountID, input.SHA1, input.Size, activeTransferStatuses).
			Updates(map[string]interface{}{
				"status":             models.PlaybackTransferTaskStatusFailed,
				"last_error_code":    interruptedCode,
				"last_error_message": interruptedMessage,
				"completed_at":       input.StartedAt,
				"updated_at":         input.StartedAt,
			}).Error; err != nil {
			return err
		}
		task = models.PlaybackTransferTask{
			SourceAccountID: input.SourceAccountID, PlaybackAccountID: input.PlaybackAccountID,
			SHA1: input.SHA1, Size: input.Size, FileName: input.FileName,
			TargetParentID: input.TargetParentID, Status: models.PlaybackTransferTaskStatusPending,
			AttemptCount: 1, StartedAt: input.StartedAt,
		}
		return tx.Create(&task).Error
	})
	if err != nil {
		return nil, safeTaskStoreError("begin_attempt", err)
	}
	return &task, nil
}

// MarkStatus advances one active task without writing Provider response text.
func (store *gormTaskStore) MarkStatus(ctx context.Context, taskID string, status models.PlaybackTransferTaskStatus, at time.Time) error {
	result := store.database(ctx).Model(&models.PlaybackTransferTask{}).
		Where("id = ? AND status IN ?", taskID, activeTransferStatuses).
		Updates(map[string]interface{}{"status": status, "updated_at": at})
	if result.Error != nil {
		return safeTaskStoreError("mark_status", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrStoreUnavailable
	}
	return nil
}

// IncrementAttempt records the one allowed challenge retry before the second
// upload initialization request is sent.
func (store *gormTaskStore) IncrementAttempt(ctx context.Context, taskID string, at time.Time) error {
	result := store.database(ctx).Model(&models.PlaybackTransferTask{}).
		Where("id = ? AND status IN ?", taskID, activeTransferStatuses).
		Updates(map[string]interface{}{
			"attempt_count": gorm.Expr("attempt_count + 1"),
			"updated_at":    at,
		})
	if result.Error != nil {
		return safeTaskStoreError("increment_attempt", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrStoreUnavailable
	}
	return nil
}

// MarkSucceeded stores reusable target provenance and access time, but never a
// signed download URL.
func (store *gormTaskStore) MarkSucceeded(ctx context.Context, taskID string, target p115integration.File, at time.Time) error {
	result := store.database(ctx).Model(&models.PlaybackTransferTask{}).
		Where("id = ? AND status IN ?", taskID, activeTransferStatuses).
		Updates(map[string]interface{}{
			"status":             models.PlaybackTransferTaskStatusSucceeded,
			"target_file_id":     target.ID,
			"target_pick_code":   target.PickCode,
			"last_error_code":    nil,
			"last_error_message": nil,
			"completed_at":       at,
			"last_accessed_at":   at,
			"updated_at":         at,
		})
	if result.Error != nil {
		return safeTaskStoreError("mark_succeeded", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrStoreUnavailable
	}
	return nil
}

// MarkFailed records only Ember-owned bounded error fields.
func (store *gormTaskStore) MarkFailed(ctx context.Context, taskID, code, message string, at time.Time) error {
	code = strings.TrimSpace(code)
	message = strings.TrimSpace(message)
	if code == "" {
		code = "unknown"
	}
	if len(code) > 100 {
		code = code[:100]
	}
	if len(message) > 500 {
		message = message[:500]
	}
	result := store.database(ctx).Model(&models.PlaybackTransferTask{}).
		Where("id = ? AND status IN ?", taskID, activeTransferStatuses).
		Updates(map[string]interface{}{
			"status":             models.PlaybackTransferTaskStatusFailed,
			"last_error_code":    code,
			"last_error_message": nullableMessage(message),
			"completed_at":       at,
			"updated_at":         at,
		})
	if result.Error != nil {
		return safeTaskStoreError("mark_failed", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrStoreUnavailable
	}
	return nil
}

// TouchSucceeded refreshes the most recent Ember-owned successful task when a
// retained file is reused. External preexisting files legitimately affect no row.
func (store *gormTaskStore) TouchSucceeded(ctx context.Context, playbackAccountID, sha1Value string, size int64, at time.Time) error {
	database := store.database(ctx)
	var task models.PlaybackTransferTask
	err := database.Select("id").
		Where("playback_account_id = ? AND sha1 = ? AND size = ? AND status = ?",
			playbackAccountID, sha1Value, size, models.PlaybackTransferTaskStatusSucceeded).
		Order("created_at DESC").Order("id DESC").First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return safeTaskStoreError("find_succeeded", err)
	}
	result := database.Model(&models.PlaybackTransferTask{}).Where("id = ?", task.ID).
		Updates(map[string]interface{}{"last_accessed_at": at, "updated_at": at})
	if result.Error != nil {
		return safeTaskStoreError("touch_succeeded", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrStoreUnavailable
	}
	return nil
}

// database disables parameter-bearing GORM diagnostics for provenance fields.
func (store *gormTaskStore) database(ctx context.Context) *gorm.DB {
	return store.db.Session(&gorm.Session{Logger: gormlogger.Default.LogMode(gormlogger.Silent)}).WithContext(ctx)
}

func nullableMessage(message string) interface{} {
	if message == "" {
		return nil
	}
	return message
}

func safeTaskStoreError(operation string, err error) error {
	if err == nil || errors.Is(err, ErrStoreUnavailable) {
		return err
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		log.Printf("[DirectPlayStore] 数据库操作失败 operation=%s code=%s constraint=%s",
			operation, pgErr.Code, pgErr.ConstraintName)
	} else {
		log.Printf("[DirectPlayStore] 存储操作失败 operation=%s errorType=%T", operation, err)
	}
	return fmt.Errorf("%w: %s", ErrStoreUnavailable, operation)
}
