package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/models"
	paymentpkg "github.com/konghang/ember/backend/internal/services/payment"
	redemptionpkg "github.com/konghang/ember/backend/internal/services/redemption"
)

type redemptionCodeService interface {
	CreateRedemptionCode(req *redemptionpkg.CreateRedemptionCodeRequest) (*models.RedemptionCode, error)
	CreateRedemptionCodesBatch(req *redemptionpkg.CreateRedemptionCodesBatchRequest) (*redemptionpkg.CreateRedemptionCodesBatchResponse, error)
	GetRedemptionCodes(req *redemptionpkg.GetRedemptionCodesRequest) (*redemptionpkg.GetRedemptionCodesResponse, error)
	DeleteRedemptionCode(id string) error
	UpdateRedemptionCode(id string, req *redemptionpkg.UpdateRedemptionCodeRequest) (*models.RedemptionCode, error)
	ValidateRegistrationCode(code string) (*models.RedemptionCode, error)
	ValidateRenewalCode(code string) (*models.RedemptionCode, error)
	GetUserTemplates() (*redemptionpkg.GetUserTemplatesResponse, error)
}

type RedemptionCodeHandler struct {
	service redemptionCodeService
}

func NewRedemptionCodeHandler() *RedemptionCodeHandler {
	return &RedemptionCodeHandler{service: &redemptionpkg.RedemptionCodeService{}}
}

func (h *RedemptionCodeHandler) CreateRedemptionCode(c *gin.Context) {
	var req redemptionpkg.CreateRedemptionCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	code, err := h.service.CreateRedemptionCode(&req)
	if err != nil {
		if isRedemptionCodeRequestError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建兑换码失败"})
		return
	}

	c.JSON(http.StatusOK, code)
}

func (h *RedemptionCodeHandler) CreateRedemptionCodesBatch(c *gin.Context) {
	var req redemptionpkg.CreateRedemptionCodesBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	resp, err := h.service.CreateRedemptionCodesBatch(&req)
	if err != nil {
		if isRedemptionCodeRequestError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "批量创建兑换码失败"})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *RedemptionCodeHandler) GetRedemptionCodes(c *gin.Context) {
	var req redemptionpkg.GetRedemptionCodesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	resp, err := h.service.GetRedemptionCodes(&req)
	if err != nil {
		if errors.Is(err, redemptionpkg.ErrRedemptionCodeStatusInvalid) ||
			errors.Is(err, paymentpkg.ErrPlanGroupInvalid) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *RedemptionCodeHandler) DeleteRedemptionCode(c *gin.Context) {
	id := c.Param("id")
	if err := h.service.DeleteRedemptionCode(id); err != nil {
		if errors.Is(err, redemptionpkg.ErrRedemptionCodeNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

func (h *RedemptionCodeHandler) UpdateRedemptionCode(c *gin.Context) {
	id := c.Param("id")

	var req redemptionpkg.UpdateRedemptionCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	code, err := h.service.UpdateRedemptionCode(id, &req)
	if err != nil {
		switch {
		case errors.Is(err, redemptionpkg.ErrRedemptionCodeNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, redemptionpkg.ErrRedemptionCodeUsedOver):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, paymentpkg.ErrPlanGroupInvalid), errors.Is(err, paymentpkg.ErrPlanGroupNotFound):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, code)
}

func (h *RedemptionCodeHandler) ValidateRegistrationCode(c *gin.Context) {
	code := c.Param("code")
	resp, err := h.service.ValidateRegistrationCode(code)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *RedemptionCodeHandler) GetUserTemplates(c *gin.Context) {
	resp, err := h.service.GetUserTemplates()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func isRedemptionCodeRequestError(err error) bool {
	return errors.Is(err, redemptionpkg.ErrRedemptionCodeBatchCountInvalid) ||
		errors.Is(err, redemptionpkg.ErrTemplateUserNotFound) ||
		errors.Is(err, redemptionpkg.ErrTemplateUserMustBeUser) ||
		errors.Is(err, redemptionpkg.ErrTemplateUserEmbyRequired) ||
		errors.Is(err, paymentpkg.ErrPlanGroupInvalid) ||
		errors.Is(err, paymentpkg.ErrPlanGroupNotFound)
}
