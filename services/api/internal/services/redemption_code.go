package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

// RedemptionCodeService 兑换码服务
type RedemptionCodeService struct{}

// CreateRedemptionCodeRequest 创建兑换码请求
type CreateRedemptionCodeRequest struct {
	MaxUses        int        `json:"maxUses" binding:"required,min=1"`
	DefaultDays    int        `json:"defaultDays" binding:"required,min=1"`
	ExpiresAt      *time.Time `json:"expiresAt"`
	TemplateUserID *string    `json:"templateUserId"`
}

// UpdateRedemptionCodeRequest 更新兑换码请求
type UpdateRedemptionCodeRequest struct {
	MaxUses        int        `json:"maxUses" binding:"required,min=1"`
	DefaultDays    int        `json:"defaultDays" binding:"required,min=1"`
	ExpiresAt      *time.Time `json:"expiresAt"`
	TemplateUserID *string    `json:"templateUserId"`
}

// GetRedemptionCodesRequest 获取兑换码列表请求
type GetRedemptionCodesRequest struct {
	Page     int  `form:"page" binding:"omitempty,min=1"`
	PageSize int  `form:"pageSize" binding:"omitempty,min=1"`
	ShowAll  bool `form:"showAll"`
}

// GetRedemptionCodesResponse 获取兑换码列表响应
type GetRedemptionCodesResponse struct {
	Data       []models.RedemptionCode `json:"data"`
	Total      int64                   `json:"total"`
	Page       int                     `json:"page"`
	PageSize   int                     `json:"pageSize"`
	TotalPages int                     `json:"totalPages"`
}

type UserTemplate struct {
	ID        string     `json:"id"`
	Username  string     `json:"username"`
	Email     string     `json:"email,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

type GetUserTemplatesResponse struct {
	Data []UserTemplate `json:"data"`
}

// CreateRedemptionCode 创建兑换码
func (s *RedemptionCodeService) CreateRedemptionCode(req *CreateRedemptionCodeRequest) (*models.RedemptionCode, error) {
	templateUserID, err := s.validateTemplateUserID(req.TemplateUserID)
	if err != nil {
		return nil, err
	}

	code, err := s.generateCode(16)
	if err != nil {
		return nil, errors.New("生成兑换码失败")
	}

	redemptionCode := models.RedemptionCode{
		Code:           code,
		MaxUses:        req.MaxUses,
		DefaultDays:    req.DefaultDays,
		ExpiresAt:      req.ExpiresAt,
		TemplateUserID: templateUserID,
	}

	if err := db.DB.Create(&redemptionCode).Error; err != nil {
		return nil, errors.New("创建兑换码失败")
	}

	return &redemptionCode, nil
}

// GetRedemptionCodes 获取兑换码列表
func (s *RedemptionCodeService) GetRedemptionCodes(req *GetRedemptionCodesRequest) (*GetRedemptionCodesResponse, error) {
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 10
	}

	var total int64
	var codes []models.RedemptionCode

	query := db.DB.Model(&models.RedemptionCode{})

	// ShowAll=false 时过滤已用完/已过期的码
	if !req.ShowAll {
		query = query.Where("\"usedCount\" < \"maxUses\" AND (\"expiresAt\" IS NULL OR \"expiresAt\" > NOW())")
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, errors.New("获取兑换码数量失败")
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Order("\"createdAt\" DESC").Offset(offset).Limit(pageSize).Find(&codes).Error; err != nil {
		return nil, errors.New("获取兑换码列表失败")
	}

	if err := s.fillTemplateUserNames(codes); err != nil {
		return nil, errors.New("获取兑换码列表失败")
	}

	return &GetRedemptionCodesResponse{
		Data:       codes,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: int(math.Ceil(float64(total) / float64(pageSize))),
	}, nil
}

// DeleteRedemptionCode 删除兑换码
func (s *RedemptionCodeService) DeleteRedemptionCode(id string) error {
	result := db.DB.Delete(&models.RedemptionCode{}, "id = ?", id)
	if result.Error != nil {
		return errors.New("删除兑换码失败")
	}
	if result.RowsAffected == 0 {
		return ErrRedemptionCodeNotFound
	}
	return nil
}

// UpdateRedemptionCode 更新兑换码
func (s *RedemptionCodeService) UpdateRedemptionCode(id string, req *UpdateRedemptionCodeRequest) (*models.RedemptionCode, error) {
	var redemptionCode models.RedemptionCode
	if err := db.DB.Where("id = ?", id).First(&redemptionCode).Error; err != nil {
		return nil, ErrRedemptionCodeNotFound
	}

	templateUserID, err := s.validateTemplateUserID(req.TemplateUserID)
	if err != nil {
		return nil, err
	}

	if req.MaxUses < redemptionCode.UsedCount {
		return nil, ErrRedemptionCodeUsedOver
	}

	redemptionCode.MaxUses = req.MaxUses
	redemptionCode.DefaultDays = req.DefaultDays
	redemptionCode.ExpiresAt = req.ExpiresAt
	redemptionCode.TemplateUserID = templateUserID

	if err := db.DB.Save(&redemptionCode).Error; err != nil {
		return nil, errors.New("更新兑换码失败")
	}

	if redemptionCode.TemplateUserID != nil && *redemptionCode.TemplateUserID != "" {
		var user models.User
		if err := db.DB.Model(&models.User{}).Select("username").Where("id = ?", *redemptionCode.TemplateUserID).First(&user).Error; err == nil {
			redemptionCode.TemplateUserName = &user.Username
		}
	}

	return &redemptionCode, nil
}

func (s *RedemptionCodeService) GetUserTemplates() (*GetUserTemplatesResponse, error) {
	now := time.Now().UTC()
	var users []models.User
	if err := db.DB.
		Model(&models.User{}).
		Where("role = ? AND \"isActive\" = true AND (\"expiresAt\" IS NULL OR \"expiresAt\" > ?)", "user", now).
		Order("username ASC").
		Find(&users).Error; err != nil {
		return nil, errors.New("获取模板用户失败")
	}

	templates := make([]UserTemplate, 0, len(users))
	for _, user := range users {
		templates = append(templates, UserTemplate{
			ID:        user.ID,
			Username:  user.Username,
			Email:     user.Email,
			ExpiresAt: user.ExpiresAt,
		})
	}

	return &GetUserTemplatesResponse{
		Data: templates,
	}, nil
}

// ValidateCode 验证兑换码（用于注册和兑换）
func (s *RedemptionCodeService) ValidateCode(code string) (*models.RedemptionCode, error) {
	var redemptionCode models.RedemptionCode
	result := db.DB.Where("code = ?", code).First(&redemptionCode)
	if result.Error != nil {
		return nil, ErrRedemptionCodeNotFound
	}

	if !redemptionCode.IsValid() {
		return nil, ErrRedemptionCodeInvalid
	}

	return &redemptionCode, nil
}

// UseCode 使用兑换码（原子递增）
func (s *RedemptionCodeService) UseCode(code string) error {
	result := db.DB.Model(&models.RedemptionCode{}).
		Where("code = ? AND \"usedCount\" < \"maxUses\"", code).
		Update("usedCount", gorm.Expr("\"usedCount\" + 1"))

	if result.Error != nil {
		return errors.New("使用兑换码失败")
	}
	if result.RowsAffected == 0 {
		return ErrRedemptionCodeInvalid
	}
	return nil
}

// generateCode 生成随机兑换码
func (s *RedemptionCodeService) generateCode(length int) (string, error) {
	bytes := make([]byte, length/2+1)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes)[:length], nil
}

func (s *RedemptionCodeService) validateTemplateUserID(templateUserID *string) (*string, error) {
	if templateUserID == nil {
		return nil, nil
	}

	if *templateUserID == "" {
		return nil, nil
	}

	var user models.User
	if err := db.DB.Where("id = ?", *templateUserID).First(&user).Error; err != nil {
		return nil, errors.New("模板用户不存在")
	}

	if user.Role != "user" {
		return nil, errors.New("模板用户必须是普通用户")
	}

	if user.EmbyID == "" {
		return nil, errors.New("模板用户未关联 Emby 账号")
	}

	validID := user.ID
	return &validID, nil
}

func (s *RedemptionCodeService) fillTemplateUserNames(codes []models.RedemptionCode) error {
	userIDs := make([]string, 0)
	seen := make(map[string]struct{})
	for i := range codes {
		if codes[i].TemplateUserID == nil || *codes[i].TemplateUserID == "" {
			continue
		}
		id := *codes[i].TemplateUserID
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		userIDs = append(userIDs, id)
	}

	if len(userIDs) == 0 {
		return nil
	}

	var users []models.User
	if err := db.DB.Model(&models.User{}).Select("id", "username").Where("id IN ?", userIDs).Find(&users).Error; err != nil {
		return err
	}

	nameMap := make(map[string]string, len(users))
	for _, user := range users {
		nameMap[user.ID] = user.Username
	}

	for i := range codes {
		if codes[i].TemplateUserID == nil || *codes[i].TemplateUserID == "" {
			continue
		}
		if name, ok := nameMap[*codes[i].TemplateUserID]; ok {
			codes[i].TemplateUserName = &name
		}
	}

	return nil
}
