package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/services"
)

type redemptionCodeService interface {
	CreateRedemptionCode(req *services.CreateRedemptionCodeRequest) (*models.RedemptionCode, error)
	CreateRedemptionCodesBatch(req *services.CreateRedemptionCodesBatchRequest) (*services.CreateRedemptionCodesBatchResponse, error)
	GetRedemptionCodes(req *services.GetRedemptionCodesRequest) (*services.GetRedemptionCodesResponse, error)
	DeleteRedemptionCode(id string) error
	UpdateRedemptionCode(id string, req *services.UpdateRedemptionCodeRequest) (*models.RedemptionCode, error)
	ValidateCode(code string) (*models.RedemptionCode, error)
	GetUserTemplates() (*services.GetUserTemplatesResponse, error)
}

type RedemptionCodeHandler struct {
	service redemptionCodeService
}

func NewRedemptionCodeHandler() *RedemptionCodeHandler {
	return &RedemptionCodeHandler{service: &services.RedemptionCodeService{}}
}

func (h *RedemptionCodeHandler) CreateRedemptionCode(c *gin.Context) {
	var req services.CreateRedemptionCodeRequest
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
	var req services.CreateRedemptionCodesBatchRequest
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
	var req services.GetRedemptionCodesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	resp, err := h.service.GetRedemptionCodes(&req)
	if err != nil {
		if errors.Is(err, services.ErrRedemptionCodeStatusInvalid) {
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
		if errors.Is(err, services.ErrRedemptionCodeNotFound) {
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

	var req services.UpdateRedemptionCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	code, err := h.service.UpdateRedemptionCode(id, &req)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrRedemptionCodeNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrRedemptionCodeUsedOver):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, code)
}

func (h *RedemptionCodeHandler) ValidateCode(c *gin.Context) {
	code := c.Param("code")
	resp, err := h.service.ValidateCode(code)
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
	return errors.Is(err, services.ErrRedemptionCodeBatchCountInvalid) ||
		errors.Is(err, services.ErrTemplateUserNotFound) ||
		errors.Is(err, services.ErrTemplateUserMustBeUser) ||
		errors.Is(err, services.ErrTemplateUserEmbyRequired)
}
