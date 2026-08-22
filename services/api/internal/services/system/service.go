package system

import (
	"context"
	"fmt"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	embytokenpkg "github.com/konghang/ember/backend/internal/services/embytoken"
	policypkg "github.com/konghang/ember/backend/internal/services/policy"
)

// SystemService 系统服务
type SystemService struct {
	embyService        *embyint.EmbyService
	now                func() time.Time
	countExpiredUsers  func(context.Context, time.Time) (int64, error)
	findExpiredUsers   func(context.Context, time.Time) ([]models.User, error)
	applyExpiredPolicy func(string) error
	revokeUserTokens   func(context.Context, string, embytokenpkg.RevokeReason, string) (int64, error)
}

// NewSystemService 创建系统服务
func NewSystemService() *SystemService {
	service := &SystemService{
		embyService: embyint.GetSharedService(),
		now:         func() time.Time { return time.Now().UTC() },
	}
	service.countExpiredUsers = countExpiredUsers
	service.findExpiredUsers = findExpiredUsers
	service.applyExpiredPolicy = func(userID string) error {
		return policypkg.NewService(service.embyService).ApplyEffectiveUserPolicyOrRecordFailure(userID, "expired_user_check")
	}
	service.revokeUserTokens = func(ctx context.Context, userID string, reason embytokenpkg.RevokeReason, actor string) (int64, error) {
		revoker, err := embytokenpkg.NewControlPlaneRevoker(db.DB)
		if err != nil {
			return 0, err
		}
		return revoker.RevokeUserTokens(ctx, userID, reason, actor)
	}
	return service
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

	if err := db.DB.Model(&models.User{}).Where("\"is_active\" = ?", true).Count(&activeUserCount).Error; err != nil {
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
