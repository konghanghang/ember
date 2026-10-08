package entitlement

import (
	"context"
	"errors"
	"fmt"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"log"
	"time"
)

type RetentionQuery func(context.Context, string, time.Time, time.Time, *time.Location) (int64, error)

// WatchRetentionWorker keeps HTTP outside transactions and revalidates state before persisting a decision.
type WatchRetentionWorker struct {
	DB       *gorm.DB
	Query    RetentionQuery
	Sync     func(string) error
	Location *time.Location
	Schedule string
}

type retentionSnapshot struct {
	User    models.User
	Holding models.UserEntitlement
	Group   models.PlanGroup
	Start   time.Time
}

// RetentionStart applies a group policy reset without reviving invalidated grants.
func RetentionStart(holding models.UserEntitlement, group models.PlanGroup) *time.Time {
	if holding.WatchRetentionInvalidatedAt != nil {
		return nil
	}
	start := holding.WatchRetentionStartedAt
	if group.WatchRetentionResetAt != nil && (start == nil || group.WatchRetentionResetAt.After(*start)) {
		start = group.WatchRetentionResetAt
	}
	return start
}

// Run examines only currently applied ordinary-user groups; inactive holdings do not consume grace.
func (w *WatchRetentionWorker) Run(ctx context.Context, at time.Time) error {
	if w.DB == nil || w.Query == nil || w.Location == nil {
		return errors.New("保号任务未配置")
	}
	var users []models.User
	if err := w.DB.WithContext(ctx).Model(&models.User{}).Joins("JOIN plan_groups g ON g.key = users.plan_group").Where("users.role = 'user' AND users.resource_access_granted = true AND g.watch_retention_enabled = true").Order("users.id").Find(&users).Error; err != nil {
		return err
	}
	log.Printf("[WatchRetention] 开始考核 candidates=%d checkedAt=%s", len(users), at.In(w.Location).Format(time.RFC3339))
	failures := 0
	for _, user := range users {
		if err := ctx.Err(); err != nil {
			return err
		}
		snapshot, err := w.snapshot(ctx, user.ID, false)
		if err != nil {
			failures++
			log.Printf("[WatchRetention] 读取失败 userId=%s errorType=%T", user.ID, err)
			continue
		}
		if snapshot == nil {
			continue
		}
		schedule := w.Schedule
		if schedule == "" {
			schedule = "0 2 * * *"
		}
		first, err := FirstWatchRetentionCheck(snapshot.Start, snapshot.Group.WatchRetentionDays, schedule, w.Location)
		if err != nil {
			failures++
			log.Printf("[WatchRetention] 调度规则无效 userId=%s", user.ID)
			continue
		}
		if at.Before(first) {
			continue
		}
		if snapshot.User.EmbyID == "" {
			log.Printf("[WatchRetention] 跳过未绑定用户 userId=%s", user.ID)
			continue
		}
		start := at.In(w.Location).AddDate(0, 0, -snapshot.Group.WatchRetentionDays)
		seconds, err := w.Query(ctx, snapshot.User.EmbyID, start, at, w.Location)
		if err != nil || seconds < 0 {
			failures++
			log.Printf("[WatchRetention] 统计失败，保留权益 userId=%s group=%s errorType=%T", user.ID, snapshot.Group.Key, err)
			continue
		}
		changed, err := w.apply(ctx, snapshot, start, at, seconds)
		if err != nil {
			failures++
			log.Printf("[WatchRetention] 考核保存失败 userId=%s errorType=%T", user.ID, err)
			continue
		}
		if changed && w.Sync != nil {
			if err := w.Sync(user.ID); err != nil {
				failures++
				log.Printf("[WatchRetention] 权限同步失败 userId=%s", user.ID)
			}
		}
	}
	if failures > 0 {
		return fmt.Errorf("观看保号存在 %d 个处理失败", failures)
	}
	return nil
}

// snapshot optionally locks the user and policy for final revalidation inside a transaction.
func (w *WatchRetentionWorker) snapshot(ctx context.Context, id string, locked bool) (*retentionSnapshot, error) {
	q := w.DB.WithContext(ctx)
	uq := q
	if locked {
		uq = uq.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var user models.User
	if err := uq.Where("id = ?", id).First(&user).Error; err != nil {
		return nil, err
	}
	if user.IsAdmin() || !user.ResourceAccessGranted || user.PlanGroup == nil {
		return nil, nil
	}
	var group models.PlanGroup
	gq := q
	if locked {
		gq = gq.Clauses(clause.Locking{Strength: "SHARE"})
	}
	if err := gq.Where("key = ?", *user.PlanGroup).First(&group).Error; err != nil {
		return nil, err
	}
	if !group.WatchRetentionEnabled {
		return nil, nil
	}
	var holding models.UserEntitlement
	if err := q.Where("user_id = ? AND plan_group = ?", user.ID, group.Key).First(&holding).Error; err != nil {
		return nil, err
	}
	start := RetentionStart(holding, group)
	if start == nil {
		return nil, nil
	}
	return &retentionSnapshot{User: user, Holding: holding, Group: group, Start: *start}, nil
}

// sameRetentionSnapshot rejects query results made stale by renewal, identity change, switching or policy edits.
func sameRetentionSnapshot(a, b *retentionSnapshot) bool {
	return a != nil && b != nil && a.User.EmbyID == b.User.EmbyID && a.Group.Key == b.Group.Key && a.Group.WatchRetentionDays == b.Group.WatchRetentionDays && a.Group.WatchRetentionMinMinutes == b.Group.WatchRetentionMinMinutes && sameTime(a.Group.WatchRetentionResetAt, b.Group.WatchRetentionResetAt) && a.Start.Equal(b.Start) && sameTime(a.Holding.ExpiresAt, b.Holding.ExpiresAt) && a.Holding.ValidityType == b.Holding.ValidityType
}

// apply atomically records the check and invalidates only its still-current grant; errors roll everything back.
func (w *WatchRetentionWorker) apply(ctx context.Context, old *retentionSnapshot, start, at time.Time, seconds int64) (bool, error) {
	changed := false
	err := w.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked := *w
		locked.DB = tx
		fresh, err := locked.snapshot(ctx, old.User.ID, true)
		if err != nil {
			return err
		}
		if !sameRetentionSnapshot(old, fresh) {
			log.Printf("[WatchRetention] 状态已变，丢弃旧统计 userId=%s", old.User.ID)
			return nil
		}
		record := models.WatchRetentionCheck{UserID: fresh.User.ID, PlanGroup: fresh.Group.Key, CheckedAt: at, WindowStart: start, StartedAt: fresh.Start, PeriodDays: fresh.Group.WatchRetentionDays, MinMinutes: fresh.Group.WatchRetentionMinMinutes, WatchedSeconds: seconds, Invalidated: seconds < int64(fresh.Group.WatchRetentionMinMinutes)*60}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		if record.Invalidated {
			if err := tx.Model(&models.UserEntitlement{}).Where("user_id = ? AND plan_group = ?", record.UserID, record.PlanGroup).Update("watch_retention_invalidated_at", at).Error; err != nil {
				return err
			}
			ranks, err := Ranks(tx)
			if err != nil {
				return err
			}
			changed, err = ReconcileLocked(tx, &fresh.User, ranks, at)
			if err != nil {
				return err
			}
		}
		log.Printf("[WatchRetention] 考核完成 userId=%s group=%s start=%s end=%s seconds=%d minMinutes=%d invalidated=%t", record.UserID, record.PlanGroup, start.Format(time.RFC3339), at.Format(time.RFC3339), seconds, record.MinMinutes, record.Invalidated)
		return nil
	})
	return changed, err
}
