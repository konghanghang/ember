package system

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
)

// CheckExpiredUsersResult 定时任务结果
type CheckExpiredUsersResult struct {
	FallbackCount    int                      `json:"fallbackCount"`
	DisabledCount    int                      `json:"disabledCount"`
	TotalExpired     int                      `json:"totalExpired"`
	Processed        int                      `json:"processed"`
	Errors           []string                 `json:"errors"`
	DisabledUsers    []DisabledUserInfo       `json:"disabledUsers,omitempty"`
	FailedUsers      []map[string]interface{} `json:"failedUsers,omitempty"`
	Canceled         bool                     `json:"canceled"`
	FailureTruncated bool                     `json:"failureTruncated,omitempty"`
}

// DisabledUserInfo 被禁用的用户信息
type DisabledUserInfo struct {
	Username  string  `json:"username"`
	Email     string  `json:"email"`
	ExpiresAt *string `json:"expiresAt"`
}

const (
	maxCheckExpiredUsersErrors      = 20
	maxCheckExpiredUsersFailedUsers = 20
)

func appendLimitedString(items []string, value string, truncated *bool, limit int) []string {
	if len(items) >= limit {
		*truncated = true
		return items
	}
	return append(items, value)
}

func appendLimitedFailedUser(items []map[string]interface{}, value map[string]interface{}, truncated *bool, limit int) []map[string]interface{} {
	if len(items) >= limit {
		*truncated = true
		return items
	}
	return append(items, value)
}

// CheckExpiredUsers 检查并禁用过期用户
func (s *SystemService) CheckExpiredUsers() (*CheckExpiredUsersResult, error) {
	return s.CheckExpiredUsersWithContext(context.Background())
}

func (s *SystemService) CheckExpiredUsersWithContext(ctx context.Context) (*CheckExpiredUsersResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return &CheckExpiredUsersResult{
				Errors:        []string{},
				DisabledUsers: []DisabledUserInfo{},
				FailedUsers:   []map[string]interface{}{},
				Canceled:      true,
			}, nil
		}
		return nil, err
	}

	errMessages := []string{}
	disabledCount := 0
	fallbackCount := 0
	processedCount := 0
	disabledUsers := []DisabledUserInfo{}
	failedUsers := []map[string]interface{}{}
	failureTruncated := false
	cutoff := s.now()

	totalExpired, err := s.countExpiredUsers(ctx, cutoff)
	if err != nil {
		return nil, fmt.Errorf("查询过期用户总数失败: %w", err)
	}

	expiredUsers, err := s.findExpiredUsers(ctx, cutoff)
	if err != nil {
		return nil, fmt.Errorf("查询过期用户失败: %w", err)
	}

	log.Printf("[Cron] 发现 %d 个待封禁过期用户", totalExpired)

	for _, user := range expiredUsers {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				log.Printf("[Cron] 过期用户检查被中断：processed=%d disabled=%d err=%v", processedCount, disabledCount, err)
				return &CheckExpiredUsersResult{
					FallbackCount:    fallbackCount,
					DisabledCount:    disabledCount,
					TotalExpired:     int(totalExpired),
					Processed:        processedCount,
					Errors:           errMessages,
					DisabledUsers:    disabledUsers,
					FailedUsers:      failedUsers,
					Canceled:         true,
					FailureTruncated: failureTruncated,
				}, nil
			}
			return nil, err
		}

		processedCount++
		current, changed, err := s.reconcileUser(ctx, user.ID, cutoff)
		if err != nil {
			errMessages = appendLimitedString(errMessages, fmt.Sprintf("重算用户 %s 权益失败", user.Username), &failureTruncated, maxCheckExpiredUsersErrors)
			log.Printf("[Cron] 权益重算失败 userId=%s err=%v", user.ID, err)
			continue
		}
		// A migrated overdue user may already have a denied local projection while
		// the old remote account is still enabled. The scheduled task must close it.
		if !changed && (current.ResourceAccessGranted || current.EmbyDisabled || current.EmbyID == "") {
			continue
		}
		if current.ResourceAccessGranted {
			if err := s.applyExpiredPolicy(current.ID); err != nil {
				errMessages = appendLimitedString(errMessages, fmt.Sprintf("用户 %s 回退权限同步失败", user.Username), &failureTruncated, maxCheckExpiredUsersErrors)
				log.Printf("[Cron] 回退同步失败 userId=%s err=%v", user.ID, err)
				continue
			}
			fallbackCount++
			log.Printf("[Cron] 用户权益已回退 userId=%s", user.ID)
			continue
		}

		if err := s.applyExpiredPolicy(user.ID); err != nil {
			errorMsg := fmt.Sprintf("禁用用户 %s 失败: %v", user.Username, err)
			errMessages = appendLimitedString(errMessages, errorMsg, &failureTruncated, maxCheckExpiredUsersErrors)
			failedUsers = appendLimitedFailedUser(failedUsers, map[string]interface{}{
				"username": user.Username,
				"error":    err.Error(),
			}, &failureTruncated, maxCheckExpiredUsersFailedUsers)
			log.Printf("[Cron] %s", errorMsg)
			continue
		}

		disabledCount++
		var expiresAtStr *string
		if user.ExpiresAt != nil {
			str := user.ExpiresAt.Format("2006-01-02 15:04:05")
			expiresAtStr = &str
		}
		disabledUsers = append(disabledUsers, DisabledUserInfo{
			Username:  user.Username,
			Email:     user.Email,
			ExpiresAt: expiresAtStr,
		})
		log.Printf("[Cron] 已禁用用户: %s (%s)", user.Username, user.ID)
	}

	log.Printf("[Cron] 定时任务完成，新封禁 %d 个，处理 %d 个过期用户", disabledCount, processedCount)

	return &CheckExpiredUsersResult{
		FallbackCount:    fallbackCount,
		DisabledCount:    disabledCount,
		TotalExpired:     int(totalExpired),
		Processed:        processedCount,
		Errors:           errMessages,
		DisabledUsers:    disabledUsers,
		FailedUsers:      failedUsers,
		FailureTruncated: failureTruncated,
	}, nil
}

// countExpiredUsers 统计需要封禁的过期用户数量。
func countExpiredUsers(ctx context.Context, cutoff time.Time) (int64, error) {
	var totalExpired int64
	err := db.DB.WithContext(ctx).Model(&models.User{}).
		Where("role = 'user' AND expires_at <= ? AND (resource_access_granted = ? OR (emby_disabled = false AND emby_id <> ''))", cutoff, true).
		Count(&totalExpired).Error
	return totalExpired, err
}

// findExpiredUsers 查询需要封禁的过期用户列表。
func findExpiredUsers(ctx context.Context, cutoff time.Time) ([]models.User, error) {
	var expiredUsers []models.User
	err := db.DB.WithContext(ctx).
		Where("role = 'user' AND expires_at <= ? AND (resource_access_granted = ? OR (emby_disabled = false AND emby_id <> ''))", cutoff, true).
		Find(&expiredUsers).Error
	return expiredUsers, err
}
