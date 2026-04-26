package system

import (
	"fmt"

	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
)

// SystemService 系统服务
type SystemService struct {
	embyService *embyint.EmbyService
}

// NewSystemService 创建系统服务
func NewSystemService() *SystemService {
	return &SystemService{
		embyService: embyint.GetSharedService(),
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

	if err := db.DB.Model(&models.User{}).Count(&userCount).Error; err != nil {
		return nil, fmt.Errorf("查询用户总数失败: %w", err)
	}

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
	_, err := s.embyService.GetUsers()
	return err
}
