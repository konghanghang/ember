package system

import (
	"fmt"
	"log"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
)

// CheckExpiredUsersResult 定时任务结果
type CheckExpiredUsersResult struct {
	DisabledCount int                      `json:"disabledCount"`
	TotalExpired  int                      `json:"totalExpired"`
	Processed     int                      `json:"processed"`
	Errors        []string                 `json:"errors"`
	DisabledUsers []DisabledUserInfo       `json:"disabledUsers,omitempty"`
	FailedUsers   []map[string]interface{} `json:"failedUsers,omitempty"`
}

// DisabledUserInfo 被禁用的用户信息
type DisabledUserInfo struct {
	Username  string  `json:"username"`
	Email     string  `json:"email"`
	ExpiresAt *string `json:"expiresAt"`
}

// CheckExpiredUsers 检查并禁用过期用户
func (s *SystemService) CheckExpiredUsers() (*CheckExpiredUsersResult, error) {
	errMessages := []string{}
	disabledCount := 0
	processedCount := 0
	disabledUsers := []DisabledUserInfo{}
	failedUsers := []map[string]interface{}{}
	cutoff := time.Now().UTC()

	var totalExpired int64
	if err := db.DB.Model(&models.User{}).
		Where("\"expiresAt\" < ? AND \"embyId\" <> '' AND \"embyDisabled\" = ?", cutoff, false).
		Count(&totalExpired).Error; err != nil {
		return nil, fmt.Errorf("查询过期用户总数失败: %w", err)
	}

	var expiredUsers []models.User
	if err := db.DB.
		Where("\"expiresAt\" < ? AND \"embyId\" <> '' AND \"embyDisabled\" = ?", cutoff, false).
		Find(&expiredUsers).Error; err != nil {
		return nil, fmt.Errorf("查询过期用户失败: %w", err)
	}

	log.Printf("[Cron] 发现 %d 个待封禁过期用户", totalExpired)

	for _, user := range expiredUsers {
		processedCount++

		err := s.embyService.SetUserPolicy(user.EmbyID, embyint.EmbyUserPolicy{
			IsDisabled: true,
		})
		if err != nil {
			errorMsg := fmt.Sprintf("禁用用户 %s 失败: %v", user.Username, err)
			errMessages = append(errMessages, errorMsg)
			failedUsers = append(failedUsers, map[string]interface{}{
				"username": user.Username,
				"error":    err.Error(),
			})
			log.Printf("[Cron] %s", errorMsg)
			continue
		}

		updateResult := db.DB.Model(&models.User{}).
			Where("id = ? AND \"embyDisabled\" = ?", user.ID, false).
			Update("\"embyDisabled\"", true)
		if updateResult.Error != nil {
			errorMsg := fmt.Sprintf("更新数据库失败 %s: %v", user.Username, updateResult.Error)
			errMessages = append(errMessages, errorMsg)
			failedUsers = append(failedUsers, map[string]interface{}{
				"username": user.Username,
				"error":    updateResult.Error.Error(),
			})
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
		DisabledCount: disabledCount,
		TotalExpired:  int(totalExpired),
		Processed:     processedCount,
		Errors:        errMessages,
		DisabledUsers: disabledUsers,
		FailedUsers:   failedUsers,
	}, nil
}
