package system

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
)

// CheckExpiredUsersResult 定时任务结果
type CheckExpiredUsersResult struct {
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

	errMessages := []string{}
	disabledCount := 0
	processedCount := 0
	disabledUsers := []DisabledUserInfo{}
	failedUsers := []map[string]interface{}{}
	failureTruncated := false
	cutoff := time.Now().UTC()

	var totalExpired int64
	if err := db.DB.WithContext(ctx).Model(&models.User{}).
		Where("\"expires_at\" < ? AND \"emby_id\" <> '' AND \"emby_disabled\" = ?", cutoff, false).
		Count(&totalExpired).Error; err != nil {
		return nil, fmt.Errorf("查询过期用户总数失败: %w", err)
	}

	var expiredUsers []models.User
	if err := db.DB.WithContext(ctx).
		Where("\"expires_at\" < ? AND \"emby_id\" <> '' AND \"emby_disabled\" = ?", cutoff, false).
		Find(&expiredUsers).Error; err != nil {
		return nil, fmt.Errorf("查询过期用户失败: %w", err)
	}

	log.Printf("[Cron] 发现 %d 个待封禁过期用户", totalExpired)

	for _, user := range expiredUsers {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				log.Printf("[Cron] 过期用户检查被中断：processed=%d disabled=%d err=%v", processedCount, disabledCount, err)
				return &CheckExpiredUsersResult{
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

		err := s.embyService.SetUserPolicy(user.EmbyID, embyint.EmbyUserPolicy{
			IsDisabled: true,
		})
		if err != nil {
			errorMsg := fmt.Sprintf("禁用用户 %s 失败: %v", user.Username, err)
			errMessages = appendLimitedString(errMessages, errorMsg, &failureTruncated, maxCheckExpiredUsersErrors)
			failedUsers = appendLimitedFailedUser(failedUsers, map[string]interface{}{
				"username": user.Username,
				"error":    err.Error(),
			}, &failureTruncated, maxCheckExpiredUsersFailedUsers)
			log.Printf("[Cron] %s", errorMsg)
			continue
		}

		updateResult := db.DB.WithContext(ctx).Model(&models.User{}).
			Where("id = ? AND \"emby_disabled\" = ?", user.ID, false).
			Update("\"emby_disabled\"", true)
		if updateResult.Error != nil {
			errorMsg := fmt.Sprintf("更新数据库失败 %s: %v", user.Username, updateResult.Error)
			errMessages = appendLimitedString(errMessages, errorMsg, &failureTruncated, maxCheckExpiredUsersErrors)
			failedUsers = appendLimitedFailedUser(failedUsers, map[string]interface{}{
				"username": user.Username,
				"error":    updateResult.Error.Error(),
			}, &failureTruncated, maxCheckExpiredUsersFailedUsers)
			log.Printf("[Cron] %s", errorMsg)
			continue
		}
		if updateResult.RowsAffected == 0 {
			log.Printf("[Cron] 用户状态已更新，跳过重复写入: %s (%s)", user.Username, user.ID)
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
		DisabledCount:    disabledCount,
		TotalExpired:     int(totalExpired),
		Processed:        processedCount,
		Errors:           errMessages,
		DisabledUsers:    disabledUsers,
		FailedUsers:      failedUsers,
		FailureTruncated: failureTruncated,
	}, nil
}
