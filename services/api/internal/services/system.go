package services

import (
	"fmt"
	"log"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
)

// SystemService 系统服务
type SystemService struct {
	embyService *EmbyService
}

// NewSystemService 创建系统服务
func NewSystemService() *SystemService {
	return &SystemService{
		embyService: NewEmbyService(),
	}
}

// SystemInfo 系统统计信息
type SystemInfo struct {
	UserCount           int64 `json:"userCount"`
	ActiveUserCount     int64 `json:"activeUserCount"`
	RedemptionCodeCount int64 `json:"redemptionCodeCount"`
}

// GetSystemInfo 获取系统信息
func (s *SystemService) GetSystemInfo() (*SystemInfo, error) {
	var userCount, activeUserCount, redemptionCodeCount int64

	// 查询用户总数
	if err := db.DB.Model(&models.User{}).Count(&userCount).Error; err != nil {
		return nil, fmt.Errorf("查询用户总数失败: %w", err)
	}

	// 查询活跃用户数
	if err := db.DB.Model(&models.User{}).Where("\"isActive\" = ?", true).Count(&activeUserCount).Error; err != nil {
		return nil, fmt.Errorf("查询活跃用户数失败: %w", err)
	}

	if err := db.DB.Model(&models.RedemptionCode{}).Count(&redemptionCodeCount).Error; err != nil {
		return nil, fmt.Errorf("查询兑换码总数失败: %w", err)
	}

	return &SystemInfo{
		UserCount:           userCount,
		ActiveUserCount:     activeUserCount,
		RedemptionCodeCount: redemptionCodeCount,
	}, nil
}

// TestEmbyConnection 测试 Emby 连接
func (s *SystemService) TestEmbyConnection() error {
	// 尝试获取用户列表来测试连接
	_, err := s.embyService.GetUsers()
	return err
}

// CheckExpiredUsersResult 定时任务结果
type CheckExpiredUsersResult struct {
	DisabledCount int                      `json:"disabledCount"`
	TotalExpired  int                      `json:"totalExpired"`
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
	errors := []string{}
	disabledCount := 0
	disabledUsers := []DisabledUserInfo{}
	failedUsers := []map[string]interface{}{}

	var expiredUsers []models.User
	err := db.DB.
		Where("\"expiresAt\" < NOW() AND \"embyDisabled\" = ?", false).
		Find(&expiredUsers).Error

	if err != nil {
		return nil, fmt.Errorf("查询过期用户失败: %w", err)
	}

	log.Printf("[Cron] 发现 %d 个过期用户", len(expiredUsers))

	// 循环处理每个过期用户
	for _, user := range expiredUsers {
		// 1. 调用 Emby API 禁用用户
		err := s.embyService.SetUserPolicy(user.EmbyID, EmbyUserPolicy{
			IsDisabled: true,
		})

		if err != nil {
			// 单个用户失败不影响其他用户
			errorMsg := fmt.Sprintf("禁用用户 %s 失败: %v", user.Username, err)
			errors = append(errors, errorMsg)
			failedUsers = append(failedUsers, map[string]interface{}{
				"username": user.Username,
				"error":    err.Error(),
			})
			log.Printf("[Cron] %s", errorMsg)
			continue
		}

		// 2. 更新数据库状态
		user.EmbyDisabled = true
		if err := db.DB.Save(&user).Error; err != nil {
			errorMsg := fmt.Sprintf("更新数据库失败 %s: %v", user.Username, err)
			errors = append(errors, errorMsg)
			failedUsers = append(failedUsers, map[string]interface{}{
				"username": user.Username,
				"error":    err.Error(),
			})
			log.Printf("[Cron] %s", errorMsg)
			continue
		}

		// 3. 记录成功
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

	log.Printf("[Cron] 定时任务完成，已禁用 %d/%d 个用户", disabledCount, len(expiredUsers))

	return &CheckExpiredUsersResult{
		DisabledCount: disabledCount,
		TotalExpired:  len(expiredUsers),
		Errors:        errors,
		DisabledUsers: disabledUsers,
		FailedUsers:   failedUsers,
	}, nil
}
