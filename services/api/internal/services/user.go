package services

import (
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
)

type userEmailVerifier interface {
	VerifyCode(email, code, codeType string) error
}

// UserService 用户服务
type UserService struct {
	emailVerifier userEmailVerifier
}

func NewUserService() *UserService {
	return NewUserServiceWithEmailVerifier(NewEmailService())
}

func NewUserServiceWithEmailVerifier(verifier userEmailVerifier) *UserService {
	service := &UserService{}
	service.setEmailVerifier(verifier)
	return service
}

func (s *UserService) setEmailVerifier(verifier userEmailVerifier) {
	if verifier == nil {
		verifier = NewEmailService()
	}
	s.emailVerifier = verifier
}

func (s *UserService) getEmailVerifier() userEmailVerifier {
	if s.emailVerifier == nil {
		s.emailVerifier = NewEmailService()
	}
	return s.emailVerifier
}

var ErrInvalidExpiresAfter = errors.New("expiresAfter 必须是 YYYY-MM-DD 格式")
var ErrInvalidEmbyStatus = errors.New("embyStatus 仅支持 available/disabled/unlinked")

func isUserExpired(expiresAt *time.Time) bool {
	if expiresAt == nil {
		return false
	}
	return expiresAt.Before(time.Now().UTC())
}

func (s *UserService) syncEmbyPolicy(user *models.User) error {
	if user.EmbyID == "" {
		return nil
	}

	shouldDisable := !user.IsActive || isUserExpired(user.ExpiresAt)
	if shouldDisable == user.EmbyDisabled {
		return nil
	}

	embyService := embyint.NewEmbyService()
	if err := embyService.SetUserPolicy(user.EmbyID, embyint.EmbyUserPolicy{IsDisabled: shouldDisable}); err != nil {
		return errors.New("同步 Emby 用户状态失败：" + err.Error())
	}

	user.EmbyDisabled = shouldDisable
	return nil
}

// AdminUpdateUserRequest 管理员更新用户请求
type AdminUpdateUserRequest struct {
	Email          *string `json:"email"`          // 邮箱（可选）
	IsActive       *bool   `json:"isActive"`       // 启用状态（可选）
	ExpiresAt      *string `json:"expiresAt"`      // 到期时间（RFC3339，可选）
	ClearExpiresAt bool    `json:"clearExpiresAt"` // 清空到期时间（置为永不过期）
}

// GetUsersRequest 获取用户列表请求
type GetUsersRequest struct {
	Page         int    `form:"page" binding:"omitempty,min=1"`     // 页码，默认 1
	PageSize     int    `form:"pageSize" binding:"omitempty,min=1"` // 每页数量，默认 20
	Search       string `form:"search"`                             // 搜索关键词（用户名/邮箱）
	IsActive     *bool  `form:"isActive"`                           // 是否启用（可选）
	ExpiresAfter string `form:"expiresAfter"`                       // 到期时间晚于该日期（YYYY-MM-DD）
	EmbyStatus   string `form:"embyStatus"`                         // Emby 状态筛选（available/disabled/unlinked）
}

// GetUsersResponse 获取用户列表响应
type GetUsersResponse struct {
	Data       []models.User `json:"data"` // 前端期望 data 字段
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
		query = query.Where("\"isActive\" = ?", *req.IsActive)
	}

	// 到期时间筛选（只返回设置了到期时间且晚于指定日期的用户）
	if req.ExpiresAfter != "" {
		expiresAfter, err := time.Parse("2006-01-02", req.ExpiresAfter)
		if err != nil {
			return nil, ErrInvalidExpiresAfter
		}
		query = query.Where("\"expiresAt\" IS NOT NULL AND \"expiresAt\" > ?", expiresAfter.UTC())
	}

	// Emby 状态筛选
	switch strings.TrimSpace(req.EmbyStatus) {
	case "":
		// 不筛选
	case "available":
		query = query.Where("COALESCE(\"embyId\", '') <> '' AND \"embyDisabled\" = ?", false)
	case "disabled":
		query = query.Where("COALESCE(\"embyId\", '') <> '' AND \"embyDisabled\" = ?", true)
	case "unlinked":
		query = query.Where("COALESCE(\"embyId\", '') = ''")
	default:
		return nil, ErrInvalidEmbyStatus
	}

	// 统计总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	// 分页查询
	var users []models.User
	offset := (req.Page - 1) * req.PageSize
	if err := query.Offset(offset).Limit(req.PageSize).Order("\"createdAt\" DESC").Find(&users).Error; err != nil {
		return nil, err
	}

	// 计算总页数
	totalPages := int(total) / req.PageSize
	if int(total)%req.PageSize > 0 {
		totalPages++
	}

	return &GetUsersResponse{
		Data:       users,
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

// UpdateUserByAdmin 管理员更新用户信息
func (s *UserService) UpdateUserByAdmin(userID string, req *AdminUpdateUserRequest) (*models.User, error) {
	if req == nil {
		return nil, errors.New("请求参数错误")
	}
	if req.Email == nil && req.IsActive == nil && req.ExpiresAt == nil && !req.ClearExpiresAt {
		return nil, errors.New("至少提供一个可更新字段")
	}
	if req.ClearExpiresAt && req.ExpiresAt != nil {
		return nil, errors.New("clearExpiresAt 和 expiresAt 不能同时设置")
	}

	var user models.User
	result := db.DB.Where("id = ?", userID).First(&user)
	if result.Error != nil {
		return nil, errors.New("用户不存在")
	}

	needSyncEmbyPolicy := false

	if req.Email != nil {
		email := strings.TrimSpace(*req.Email)
		if email == "" {
			return nil, errors.New("邮箱不能为空")
		}
		if _, err := mail.ParseAddress(email); err != nil {
			return nil, errors.New("邮箱格式错误")
		}
		user.Email = email
	}

	if req.IsActive != nil {
		user.IsActive = *req.IsActive
		needSyncEmbyPolicy = true
	}

	if req.ClearExpiresAt {
		user.ExpiresAt = nil
		needSyncEmbyPolicy = true
	} else if req.ExpiresAt != nil {
		expiresAt, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			return nil, errors.New("expiresAt 必须是 RFC3339 格式")
		}
		expiresAtUTC := expiresAt.UTC()
		user.ExpiresAt = &expiresAtUTC
		needSyncEmbyPolicy = true
	}

	if needSyncEmbyPolicy {
		if err := s.syncEmbyPolicy(&user); err != nil {
			return nil, err
		}
	}

	if err := db.DB.Save(&user).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key value") && strings.Contains(err.Error(), "email") {
			return nil, errors.New("邮箱已存在")
		}
		return nil, errors.New("更新失败")
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
	now := time.Now().UTC()
	if user.ExpiresAt == nil || user.ExpiresAt.Before(now) {
		// 如果未设置过期或已过期，从当前时间开始计算
		newExpiry = now.AddDate(0, 0, days)
	} else {
		// 从原到期时间延长
		newExpiry = user.ExpiresAt.AddDate(0, 0, days)
	}

	user.ExpiresAt = &newExpiry

	if err := s.syncEmbyPolicy(&user); err != nil {
		return nil, err
	}

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

	if err := s.syncEmbyPolicy(&user); err != nil {
		return nil, err
	}

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

	// 先删除 Emby 用户，避免本地删除成功但 Emby 残留
	if user.EmbyID != "" {
		embyService := embyint.NewEmbyService()
		if err := embyService.DeleteUser(user.EmbyID); err != nil {
			return errors.New("删除用户失败：" + err.Error())
		}
	}

	// 软删除（如果需要硬删除，使用 Unscoped()）
	if err := db.DB.Delete(&user).Error; err != nil {
		return err
	}

	return nil
}
