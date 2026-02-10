package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/services"
)

// InviteHandler 邀请码处理器
type InviteHandler struct {
	inviteService *services.InviteService
}

// NewInviteHandler 创建邀请码处理器
func NewInviteHandler() *InviteHandler {
	return &InviteHandler{
		inviteService: &services.InviteService{},
	}
}

// CreateInvite 创建邀请码
// @Summary 创建邀请码
// @Tags 邀请码管理
// @Accept json
// @Produce json
// @Param body body services.CreateInviteRequest true "邀请码信息"
// @Success 200 {object} models.Invite
// @Router /api/v1/admin/invites [post]
// @Security BearerAuth
func (h *InviteHandler) CreateInvite(c *gin.Context) {
	var req services.CreateInviteRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误",
		})
		return
	}

	invite, err := h.inviteService.CreateInvite(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, invite)
}

// GetInvites 获取邀请码列表
// @Summary 获取邀请码列表
// @Tags 邀请码管理
// @Produce json
// @Param page query int false "页码" default(1)
// @Param pageSize query int false "每页数量" default(20)
// @Param showAll query bool false "是否显示全部" default(false)
// @Success 200 {object} services.GetInvitesResponse
// @Router /api/v1/admin/invites [get]
// @Security BearerAuth
func (h *InviteHandler) GetInvites(c *gin.Context) {
	var req services.GetInvitesRequest

	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数错误",
		})
		return
	}

	resp, err := h.inviteService.GetInvites(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// DeleteInvite 删除邀请码
// @Summary 删除邀请码
// @Tags 邀请码管理
// @Produce json
// @Param id path string true "邀请码ID"
// @Success 200 {object} SuccessResponse
// @Router /api/v1/admin/invites/{id} [delete]
// @Security BearerAuth
func (h *InviteHandler) DeleteInvite(c *gin.Context) {
	inviteID := c.Param("id")

	err := h.inviteService.DeleteInvite(inviteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "删除成功",
	})
}

// ValidateInvite 验证邀请码
// @Summary 验证邀请码
// @Tags 邀请码管理
// @Produce json
// @Param code path string true "邀请码"
// @Success 200 {object} models.Invite
// @Router /api/v1/invites/{code}/validate [get]
func (h *InviteHandler) ValidateInvite(c *gin.Context) {
	code := c.Param("code")

	invite, err := h.inviteService.ValidateInvite(code)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, invite)
}
