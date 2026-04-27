package telegram

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

const pendingRejectTTL = 5 * time.Minute

// EnqueuePendingReject 创建一条待确认拒绝记录
func EnqueuePendingReject(ctx context.Context, chatID int64, adminUserID, subscriptionID string, messageID *int64, hasPhoto bool, originalText string) error {
	now := time.Now().UTC()
	record := &models.BotPendingRejectRequest{
		ChatID:         chatID,
		AdminUserID:    adminUserID,
		SubscriptionID: subscriptionID,
		MessageID:      messageID,
		HasPhoto:       hasPhoto,
		OriginalText:   originalText,
		ExpiresAt:      now.Add(pendingRejectTTL),
	}
	if err := db.DB.WithContext(ctx).Create(record).Error; err != nil {
		return err
	}
	log.Printf("[PendingReject] 入队成功 chatId=%d adminUserId=%s subscriptionId=%s", chatID, adminUserID, subscriptionID)
	return nil
}

// PopPendingReject 原子性地取出并删除最新未过期的拒绝待确认记录
func PopPendingReject(ctx context.Context, chatID int64) (*models.BotPendingRejectRequest, error) {
	var record models.BotPendingRejectRequest
	err := db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.
			Where(`"chatId" = ? AND "expiresAt" > ?`, chatID, time.Now().UTC()).
			Order(`"createdAt" DESC`).
			First(&record).Error
		if err != nil {
			return err
		}
		return tx.Delete(&record).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

// CleanupExpiredPendingRejects 清理过期的待确认拒绝记录
func CleanupExpiredPendingRejects(ctx context.Context) (int64, error) {
	result := db.DB.WithContext(ctx).
		Where(`"expiresAt" < ?`, time.Now().UTC()).
		Delete(&models.BotPendingRejectRequest{})
	return result.RowsAffected, result.Error
}
