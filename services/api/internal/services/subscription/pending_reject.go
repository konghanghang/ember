package subscription

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/konghang/ember/backend/internal/async"
	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PendingRejectCompleteResult 描述 Bot 拒绝原因提交后的订阅权威终态。
type PendingRejectCompleteResult struct {
	SubscriptionID string                    `json:"subscriptionId"`
	Status         models.SubscriptionStatus `json:"status"`
	Changed        bool                      `json:"changed"`
	RejectReason   string                    `json:"rejectReason"`
}

var runSubscriptionRejectedNotifications = func(s *SubscriptionService, subscription models.Subscription) {
	async.SafeGo("subscription.notifyRejected", func() { s.notifyRejected(subscription) })
	async.SafeGo("subscription.syncAdminRejected", func() { s.syncAdminNotifications(subscription) })
}

// CompletePendingReject 使用持久化 pending request 完成 Bot 拒绝流程。
//
// 方法在同一事务内锁定 pending request 和订阅记录，只允许匹配 chat/admin 且未过期的
// request 修改 PENDING 订阅。已进入终态的订阅只回放权威状态，不重复通知。
func (s *SubscriptionService) CompletePendingReject(ctx context.Context, pendingRequestID string, chatID int64, adminUserID, reason string) (*PendingRejectCompleteResult, error) {
	pendingRequestID = strings.TrimSpace(pendingRequestID)
	adminUserID = strings.TrimSpace(adminUserID)
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, ErrSubscriptionRejectReason
	}
	if pendingRequestID == "" || adminUserID == "" {
		return nil, ErrPendingRejectNotFound
	}

	var completed models.Subscription
	var response *PendingRejectCompleteResult
	err := db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var pending models.BotPendingRejectRequest
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where(`"id" = ? AND "chat_id" = ? AND "admin_user_id" = ?`, pendingRequestID, chatID, adminUserID).
			First(&pending).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPendingRejectNotFound
			}
			return fmt.Errorf("查询拒绝待确认记录失败: %w", err)
		}

		var subscription models.Subscription
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where(`"id" = ?`, pending.SubscriptionID).
			First(&subscription).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPendingRejectNotFound
			}
			return fmt.Errorf("查询订阅失败: %w", err)
		}

		now := subscriptionNow()
		if !pending.ExpiresAt.After(now) {
			return ErrPendingRejectNotFound
		}

		response = &PendingRejectCompleteResult{
			SubscriptionID: subscription.ID,
			Status:         subscription.Status,
			RejectReason:   stringPointerValue(subscription.RejectReason),
		}
		switch subscription.Status {
		case models.SubscriptionRejected:
			return nil
		case models.SubscriptionApproved, models.SubscriptionIngested:
			return nil
		case models.SubscriptionPending:
			result := tx.Model(&models.Subscription{}).
				Where(`"id" = ? AND "status" = ?`, subscription.ID, models.SubscriptionPending).
				Updates(map[string]interface{}{
					"status":        models.SubscriptionRejected,
					"reviewed_at":   now,
					"reject_reason": reason,
					"review_source": nil,
				})
			if result.Error != nil {
				return fmt.Errorf("更新订阅状态失败: %w", result.Error)
			}
			if result.RowsAffected == 0 {
				return ErrSubscriptionStateConflict
			}
			subscription.Status = models.SubscriptionRejected
			subscription.ReviewedAt = &now
			subscription.RejectReason = &reason
			completed = subscription
			response.Status = models.SubscriptionRejected
			response.Changed = true
			response.RejectReason = reason
			return nil
		default:
			return ErrSubscriptionHandled
		}
	})
	if err != nil {
		return nil, err
	}
	if response != nil && response.Changed {
		log.Printf("[Subscription] Bot 拒绝完成 pendingRequestId=%s subscriptionId=%s adminUserId=%s",
			pendingRequestID, response.SubscriptionID, adminUserID)
		runSubscriptionRejectedNotifications(s, completed)
	}
	return response, nil
}
