package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
)

// InviteService 邀请码服务
type InviteService struct{}

// CreateInviteRequest 创建邀请码请求
type CreateInviteRequest struct {
	MaxUses     int        `json:"maxUses" binding:"required,min=1"`     // 最大使用次数
	DefaultDays int        `json:"defaultDays" binding:"required,min=1"` // 默认有效天数
	ExpiresAt   *time.Time `json:"expiresAt"`                            // 邀请码过期时间（可选）
}

// GetInvitesRequest 获取邀请码列表请求
type GetInvitesRequest struct {
	Page     int  `form:"page" binding:"omitempty,min=1"`
	PageSize int  `form:"pageSize" binding:"omitempty,min=1"`
	ShowAll  bool `form:"showAll"` // 是否显示所有（包括已过期和已用完）
}

// GetInvitesResponse 获取邀请码列表响应
type GetInvitesResponse struct {
	Invites    []models.Invite `json:"invites"`
	Total      int64           `json:"total"`
	Page       int             `json:"page"`
	PageSize   int             `json:"pageSize"`
	TotalPages int             `json:"totalPages"`
}

// CreateInvite 创建邀请码
func (s *InviteService) CreateInvite(req *CreateInviteRequest) (*models.Invite, error) {
	// 生成随机邀请码（8 字节 = 16 字符）
	code, err := generateInviteCode(8)
	if err != nil {
		return nil, errors.New("生成邀请码失败")
	}

	// 创建邀请码
	invite := models.Invite{
		Code:        code,
		MaxUses:     req.MaxUses,
		UsedCount:   0,
		DefaultDays: req.DefaultDays,
		ExpiresAt:   req.ExpiresAt,
	}

	// 保存到数据库
	if err := db.DB.Create(&invite).Error; err != nil {
		return nil, errors.New("创建邀请码失败")
	}

	return &invite, nil
}

// GetInvites 获取邀请码列表
func (s *InviteService) GetInvites(req *GetInvitesRequest) (*GetInvitesResponse, error) {
	// 设置默认值
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}

	// 构建查询
	query := db.DB.Model(&models.Invite{})

	// 如果不显示全部，只显示有效的邀请码
	if !req.ShowAll {
		now := time.Now()
		query = query.Where("used_count < max_uses").
			Where("(expires_at IS NULL OR expires_at > ?)", now)
	}

	// 统计总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	// 分页查询
	var invites []models.Invite
	offset := (req.Page - 1) * req.PageSize
	if err := query.Offset(offset).Limit(req.PageSize).Order("created_at DESC").Find(&invites).Error; err != nil {
		return nil, err
	}

	// 计算总页数
	totalPages := int(total) / req.PageSize
	if int(total)%req.PageSize > 0 {
		totalPages++
	}

	return &GetInvitesResponse{
		Invites:    invites,
		Total:      total,
		Page:       req.Page,
		PageSize:   req.PageSize,
		TotalPages: totalPages,
	}, nil
}

// DeleteInvite 删除邀请码
func (s *InviteService) DeleteInvite(inviteID string) error {
	var invite models.Invite
	result := db.DB.Where("id = ?", inviteID).First(&invite)
	if result.Error != nil {
		return errors.New("邀请码不存在")
	}

	// 删除邀请码
	if err := db.DB.Delete(&invite).Error; err != nil {
		return errors.New("删除邀请码失败")
	}

	return nil
}

// ValidateInvite 验证邀请码
func (s *InviteService) ValidateInvite(code string) (*models.Invite, error) {
	var invite models.Invite
	result := db.DB.Where("code = ?", code).First(&invite)
	if result.Error != nil {
		return nil, errors.New("邀请码不存在")
	}

	// 检查是否有效
	if !invite.IsValid() {
		return nil, errors.New("邀请码已失效")
	}

	return &invite, nil
}

// UseInvite 使用邀请码（增加使用次数）
func (s *InviteService) UseInvite(code string) error {
	var invite models.Invite
	result := db.DB.Where("code = ?", code).First(&invite)
	if result.Error != nil {
		return errors.New("邀请码不存在")
	}

	// 检查是否有效
	if !invite.IsValid() {
		return errors.New("邀请码已失效")
	}

	// 增加使用次数
	invite.UsedCount++

	// 更新数据库
	if err := db.DB.Save(&invite).Error; err != nil {
		return errors.New("使用邀请码失败")
	}

	return nil
}

// generateInviteCode 生成随机邀请码
func generateInviteCode(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
