package redemption

import (
	"time"

	"github.com/konghang/ember/backend/internal/models"
)

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
	Username string `form:"username"`
	Code     string `form:"code"`
}

type GetAllRedemptionsResponse struct {
	Data       []RedemptionWithUser `json:"data"`
	Total      int64                `json:"total"`
	Page       int                  `json:"page"`
	PageSize   int                  `json:"pageSize"`
	TotalPages int                  `json:"totalPages"`
}

type RedemptionCodeStatus string

const (
	RedemptionCodeStatusActive    RedemptionCodeStatus = "active"
	RedemptionCodeStatusExpired   RedemptionCodeStatus = "expired"
	RedemptionCodeStatusExhausted RedemptionCodeStatus = "exhausted"
)

type RedemptionCodeCreateOptions struct {
	MaxUses        int        `json:"maxUses" binding:"required,min=1"`
	DefaultDays    int        `json:"defaultDays" binding:"required,min=1"`
	ExpiresAt      *time.Time `json:"expiresAt"`
	TemplateUserID *string    `json:"templateUserId"`
	Notes          string     `json:"notes" binding:"omitempty,max=500"`
}

type CreateRedemptionCodeRequest struct {
	RedemptionCodeCreateOptions
}

type CreateRedemptionCodesBatchRequest struct {
	Count int `json:"count" binding:"required,min=1,max=100"`
	RedemptionCodeCreateOptions
}

type CreateRedemptionCodesBatchResponse struct {
	Data  []models.RedemptionCode `json:"data"`
	Count int                     `json:"count"`
}

type UpdateRedemptionCodeRequest struct {
	MaxUses        int        `json:"maxUses" binding:"required,min=1"`
	DefaultDays    int        `json:"defaultDays" binding:"required,min=1"`
	ExpiresAt      *time.Time `json:"expiresAt"`
	TemplateUserID *string    `json:"templateUserId"`
	Notes          string     `json:"notes" binding:"omitempty,max=500"`
}

type GetRedemptionCodesRequest struct {
	Page           int    `form:"page" binding:"omitempty,min=1"`
	PageSize       int    `form:"pageSize" binding:"omitempty,min=1"`
	ShowAll        bool   `form:"showAll"`
	Code           string `form:"code"`
	Status         string `form:"status"`
	TemplateUserID string `form:"templateUserId"`
}

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
