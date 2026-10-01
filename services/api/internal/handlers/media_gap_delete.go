package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/services/mediagap"
)

type mediaGapDeleteRequest struct {
	IDs []string `json:"ids"`
}

// DeleteMediaGap 删除单条已收口工单，沿用批量服务的事务与状态保护。
func (h *MediaGapHandler) DeleteMediaGap(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		writeMediaGapError(c, mediagap.ErrMediaGapDeleteIDs)
		return
	}
	h.deleteMediaGaps(c, []string{id})
}

// BatchDeleteMediaGaps 绑定显式 ID 列表；不提供按筛选条件清空接口。
func (h *MediaGapHandler) BatchDeleteMediaGaps(c *gin.Context) {
	var req mediaGapDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	h.deleteMediaGaps(c, req.IDs)
}

// deleteMediaGaps 拼装统一删除结果；计数仅在事务提交后返回。
func (h *MediaGapHandler) deleteMediaGaps(c *gin.Context, ids []string) {
	count, err := h.service.DeleteClosedGaps(c.Request.Context(), ids)
	if err != nil {
		writeMediaGapError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"deletedCount": count, "message": "缺集工单已删除"}})
}
