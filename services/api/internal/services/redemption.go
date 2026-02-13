package services

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

// RedemptionService 兑换服务
type RedemptionService struct{}

type RedeemCodeRequest struct {
	Code string `json:"code" binding:"required"`
}

type RedeemCodeResponse struct {
	Message   string     `json:"message"`
	Days      int        `json:"days"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

type GetRedemptionsRequest struct {
	Page     int `form:"page" binding:"omitempty,min=1"`
	PageSize int `form:"pageSize" binding:"omitempty,min=1"`
}

type GetRedemptionsResponse struct {
	Data       []models.Redemption `json:"data"`
	Total      int64               `json:"total"`
	Page       int                 `json:"page"`
	PageSize   int                 `json:"pageSize"`
	TotalPages int                 `json:"totalPages"`
}

type RedemptionWithUser struct {
	models.Redemption
	Username string `json:"username"`
}

type GetAllRedemptionsRequest struct {
	Page     int    `form:"page" binding:"omitempty,min=1"`
	PageSize int    `form:"pageSize" binding:"omitempty,min=1"`
	UserID   string `form:"userId"`
}

type GetAllRedemptionsResponse struct {
	Data       []RedemptionWithUser `json:"data"`
	Total      int64                `json:"total"`
	Page       int                  `json:"page"`
	PageSize   int                  `json:"pageSize"`
	TotalPages int                  `json:"totalPages"`
}

// RedeemCode 兑换续期
func (s *RedemptionService) RedeemCode(userID string, req *RedeemCodeRequest) (*RedeemCodeResponse, error) {
	codeService := &RedemptionCodeService{}
	code, err := codeService.ValidateCode(req.Code)
	if err != nil {
		return nil, err
	}

	var user models.User
	if err := db.DB.Where("id = ?", userID).First(&user).Error; err != nil {
		return nil, errors.New("用户不存在")
	}

	now := time.Now().UTC()
	var newExpiry time.Time
	if user.ExpiresAt == nil || user.ExpiresAt.Before(now) {
		newExpiry = now.AddDate(0, 0, code.DefaultDays)
	} else {
		newExpiry = user.ExpiresAt.AddDate(0, 0, code.DefaultDays)
	}

	tx := db.DB.Begin()
	if tx.Error != nil {
		return nil, ErrRedeemFailed
	}

	user.ExpiresAt = &newExpiry

	if user.EmbyDisabled && user.IsActive {
		embyService := NewEmbyService()
		if err := embyService.SetUserPolicy(user.EmbyID, EmbyUserPolicy{IsDisabled: false}); err != nil {
			tx.Rollback()
			return nil, ErrEmbyUnbanFailed
		}
		user.EmbyDisabled = false
	}

	if err := tx.Save(&user).Error; err != nil {
		tx.Rollback()
		return nil, ErrRedeemFailed
	}

	result := tx.Model(&models.RedemptionCode{}).
		Where("code = ? AND \"usedCount\" < \"maxUses\"", req.Code).
		Update("usedCount", gorm.Expr("\"usedCount\" + 1"))
	if result.Error != nil {
		tx.Rollback()
		return nil, ErrRedeemFailed
	}
	if result.RowsAffected == 0 {
		tx.Rollback()
		return nil, ErrRedemptionCodeInvalid
	}

	if err := tx.Create(&models.Redemption{
		UserID: user.ID,
		Code:   req.Code,
		Days:   code.DefaultDays,
	}).Error; err != nil {
		tx.Rollback()
		return nil, ErrRedeemFailed
	}

	if err := tx.Commit().Error; err != nil {
		return nil, ErrRedeemFailed
	}

	return &RedeemCodeResponse{
		Message:   fmt.Sprintf("兑换成功，有效期已延长 %d 天", code.DefaultDays),
		Days:      code.DefaultDays,
		ExpiresAt: &newExpiry,
	}, nil
}

func (s *RedemptionService) GetRedemptions(userID string, req *GetRedemptionsRequest) (*GetRedemptionsResponse, error) {
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 10
	}

	query := db.DB.Model(&models.Redemption{}).Where("\"userId\" = ?", userID)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, errors.New("获取兑换记录失败")
	}

	var rows []models.Redemption
	offset := (page - 1) * pageSize
	if err := query.Order("\"createdAt\" DESC").Offset(offset).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, errors.New("获取兑换记录失败")
	}

	return &GetRedemptionsResponse{
		Data:       rows,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: int(math.Ceil(float64(total) / float64(pageSize))),
	}, nil
}

func (s *RedemptionService) GetAllRedemptions(req *GetAllRedemptionsRequest) (*GetAllRedemptionsResponse, error) {
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 10
	}

	base := db.DB.Table("redemptions r").Joins("LEFT JOIN users u ON r.\"userId\" = u.id")
	if req.UserID != "" {
		base = base.Where("r.\"userId\" = ?", req.UserID)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, errors.New("获取兑换记录失败")
	}

	var rows []RedemptionWithUser
	offset := (page - 1) * pageSize
	if err := base.
		Select("r.id, r.\"userId\", r.code, r.days, r.\"createdAt\", COALESCE(u.username, '') AS username").
		Order("r.\"createdAt\" DESC").
		Offset(offset).
		Limit(pageSize).
		Scan(&rows).Error; err != nil {
		return nil, errors.New("获取兑换记录失败")
	}

	return &GetAllRedemptionsResponse{
		Data:       rows,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: int(math.Ceil(float64(total) / float64(pageSize))),
	}, nil
}
