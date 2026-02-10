package services

import (
	"errors"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
)

// UserService 用户服务
type UserService struct{}

// GetUsersRequest 获取用户列表请求
type GetUsersRequest struct {
	Page     int    `form:"page" binding:"omitempty,min=1"`     // 页码，默认 1
	PageSize int    `form:"pageSize" binding:"omitempty,min=1"` // 每页数量，默认 20
	Search   string `form:"search"`                             // 搜索关键词（用户名/邮箱）
	IsActive *bool  `form:"isActive"`                           // 是否启用（可选）
}

// GetUsersResponse 获取用户列表响应
type GetUsersResponse struct {
	Users      []models.User `json:"users"`
	Total      int64         `json:"total"`
	Page       int           `json:"page"`
	PageSize   int           `json:"pageSize"`
	TotalPages int           `json:"totalPages"`
}

// GetUsers 获取用户列表
func (s *UserService) GetUsers(req *GetUsersRequest) (*GetUsersResponse, error) {
	// 设置默认值
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}

	// 构建查询
	query := db.DB.Model(&models.User{})

	// 搜索条件
	if req.Search != "" {
		query = query.Where("username LIKE ? OR email LIKE ?", "%"+req.Search+"%", "%"+req.Search+"%")
	}

	// 是否启用筛选
	if req.IsActive != nil {
		query = query.Where("is_active = ?", *req.IsActive)
	}

	// 统计总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	// 分页查询
	var users []models.User
	offset := (req.Page - 1) * req.PageSize
	if err := query.Offset(offset).Limit(req.PageSize).Order("created_at DESC").Find(&users).Error; err != nil {
		return nil, err
	}

	// 计算总页数
	totalPages := int(total) / req.PageSize
	if int(total)%req.PageSize > 0 {
		totalPages++
	}

	return &GetUsersResponse{
		Users:      users,
		Total:      total,
		Page:       req.Page,
		PageSize:   req.PageSize,
		TotalPages: totalPages,
	}, nil
}

// GetUserByID 获取用户详情
func (s *UserService) GetUserByID(userID string) (*models.User, error) {
	var user models.User
	result := db.DB.Where("id = ?", userID).First(&user)
	if result.Error != nil {
		return nil, errors.New("用户不存在")
	}
	return &user, nil
}

// ExtendExpiryRequest 延长到期时间请求
type ExtendExpiryRequest struct {
	Days int `json:"days" binding:"required,min=1"` // 延长天数
}

// ExtendExpiry 延长用户到期时间
func (s *UserService) ExtendExpiry(userID string, days int) (*models.User, error) {
	var user models.User
	result := db.DB.Where("id = ?", userID).First(&user)
	if result.Error != nil {
		return nil, errors.New("用户不存在")
	}

	// 计算新的到期时间
	var newExpiry time.Time
	if user.ExpiresAt == nil || user.ExpiresAt.Before(time.Now()) {
		// 如果未设置过期或已过期，从当前时间开始计算
		newExpiry = time.Now().AddDate(0, 0, days)
	} else {
		// 从原到期时间延长
		newExpiry = user.ExpiresAt.AddDate(0, 0, days)
	}

	user.ExpiresAt = &newExpiry

	// 更新数据库
	if err := db.DB.Save(&user).Error; err != nil {
		return nil, err
	}

	return &user, nil
}

// ToggleUserStatus 启用/禁用用户
func (s *UserService) ToggleUserStatus(userID string) (*models.User, error) {
	var user models.User
	result := db.DB.Where("id = ?", userID).First(&user)
	if result.Error != nil {
		return nil, errors.New("用户不存在")
	}

	// 切换状态
	user.IsActive = !user.IsActive

	// 更新数据库
	if err := db.DB.Save(&user).Error; err != nil {
		return nil, err
	}

	return &user, nil
}

// DeleteUser 删除用户
func (s *UserService) DeleteUser(userID string) error {
	var user models.User
	result := db.DB.Where("id = ?", userID).First(&user)
	if result.Error != nil {
		return errors.New("用户不存在")
	}

	// 软删除（如果需要硬删除，使用 Unscoped()）
	if err := db.DB.Delete(&user).Error; err != nil {
		return err
	}

	return nil
}

// ResetPasswordRequest 重置密码请求
type ResetPasswordRequest struct {
	NewPassword string `json:"newPassword" binding:"required,min=6"`
}

// ResetPassword 重置用户密码（管理员操作）
// TODO: 需要调用 Emby API 重置密码
func (s *UserService) ResetPassword(userID string, newPassword string) error {
	var user models.User
	result := db.DB.Where("id = ?", userID).First(&user)
	if result.Error != nil {
		return errors.New("用户不存在")
	}

	// TODO: 调用 Emby API 重置密码
	// embyService := &EmbyService{}
	// err := embyService.ResetUserPassword(user.EmbyID, newPassword)
	// if err != nil {
	//     return err
	// }

	return nil
}
