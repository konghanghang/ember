package telegram

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	botPollingLockName    = "telegram_polling"
	minBotPollingLeaseSec = 15
	maxBotPollingLeaseSec = 600
)

var ErrBotPollingLockHeld = errors.New("Bot polling 锁已被其他实例持有")
var ErrBotPollingLockLost = errors.New("Bot polling 锁不存在或已失效")

func normalizeBotPollingLeaseSeconds(value int) int {
	if value < minBotPollingLeaseSec {
		return minBotPollingLeaseSec
	}
	if value > maxBotPollingLeaseSec {
		return maxBotPollingLeaseSec
	}
	return value
}

func AcquireBotPollingLock(ctx context.Context, ownerID string, leaseSeconds int) error {
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return errors.New("ownerId 不能为空")
	}

	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(normalizeBotPollingLeaseSeconds(leaseSeconds)) * time.Second)

	return db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var lock models.BotRuntimeLock
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("name = ?", botPollingLockName).
			First(&lock).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return tx.Create(&models.BotRuntimeLock{
				Name:      botPollingLockName,
				OwnerID:   ownerID,
				ExpiresAt: expiresAt,
			}).Error
		case err != nil:
			return err
		}

		if lock.OwnerID != ownerID && lock.ExpiresAt.After(now) {
			return ErrBotPollingLockHeld
		}

		return tx.Model(&models.BotRuntimeLock{}).
			Where("name = ?", botPollingLockName).
			Updates(map[string]interface{}{
				"owner_id":   ownerID,
				"expires_at": expiresAt,
				"updated_at": now,
			}).Error
	})
}

func RenewBotPollingLock(ctx context.Context, ownerID string, leaseSeconds int) error {
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return errors.New("ownerId 不能为空")
	}

	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(normalizeBotPollingLeaseSeconds(leaseSeconds)) * time.Second)

	result := db.DB.WithContext(ctx).Model(&models.BotRuntimeLock{}).
		Where("name = ? AND \"owner_id\" = ? AND \"expires_at\" > ?", botPollingLockName, ownerID, now).
		Updates(map[string]interface{}{
			"expires_at": expiresAt,
			"updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrBotPollingLockLost
	}
	return nil
}

func ReleaseBotPollingLock(ctx context.Context, ownerID string) error {
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return errors.New("ownerId 不能为空")
	}

	result := db.DB.WithContext(ctx).
		Where("name = ? AND \"owner_id\" = ?", botPollingLockName, ownerID).
		Delete(&models.BotRuntimeLock{})
	if result.Error != nil {
		return result.Error
	}
	return nil
}
