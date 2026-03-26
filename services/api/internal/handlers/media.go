package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	mediapkg "github.com/konghang/ember/backend/internal/services/media"
)

// MediaHandler 媒体处理器
type MediaHandler struct {
	service *mediapkg.MediaService
}

// NewMediaHandler 创建媒体处理器
func NewMediaHandler() *MediaHandler {
	return &MediaHandler{
		service: mediapkg.NewMediaService(),
	}
}

// GetEmbyConfig 获取 Emby 配置
// GET /api/v1/emby/config
func (h *MediaHandler) GetEmbyConfig(c *gin.Context) {
	url, err := h.service.GetEmbyConfig()
	if err != nil || url == "" {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "未配置 Emby 服务器地址",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"url":     url,
	})
}

// GetMediaStats 获取媒体库统计信息（带缓存）
// GET /api/v1/media/stats
func (h *MediaHandler) GetMediaStats(c *gin.Context) {
	stats, err := h.service.GetMediaStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

type LatestMediaItem struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Type            string   `json:"type"`
	ProductionYear  int      `json:"productionYear"`
	DateCreated     string   `json:"dateCreated"`
	CommunityRating *float64 `json:"communityRating,omitempty"`
	OfficialRating  *string  `json:"officialRating,omitempty"`
	Overview        *string  `json:"overview,omitempty"`
	ChildCount      int      `json:"childCount"`
}

func normalizeEmbyTime(raw string) string {
	if raw == "" {
		return ""
	}
	// Emby 常见格式为 RFC3339Nano（例如 2024-01-15T10:30:00.0000000Z）
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	return raw
}

// GetLatestItems 获取最近入库的媒体
// GET /api/v1/media/latest?type=Movie&limit=20
func (h *MediaHandler) GetLatestItems(c *gin.Context) {
	userIDRaw, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "未认证"})
		return
	}

	userID, ok := userIDRaw.(string)
	if !ok || userID == "" {
		userID = fmt.Sprint(userIDRaw)
	}
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "未认证"})
		return
	}

	var user models.User
	if err := db.DB.Select("embyId").Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "用户不存在"})
		return
	}
	if user.EmbyID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "用户未绑定 Emby 账号"})
		return
	}

	itemType := c.DefaultQuery("type", "Movie") // Movie / Series
	if itemType != "Movie" && itemType != "Series" {
		itemType = "Movie"
	}

	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit <= 0 || limit > 50 {
		limit = 20
	}

	items, err := h.service.GetLatestItems(user.EmbyID, itemType, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	res := make([]LatestMediaItem, 0, len(items))
	for _, it := range items {
		res = append(res, LatestMediaItem{
			ID:              it.ID,
			Name:            it.Name,
			Type:            it.Type,
			ProductionYear:  it.ProductionYear,
			DateCreated:     normalizeEmbyTime(it.DateCreated),
			CommunityRating: it.CommunityRating,
			OfficialRating:  it.OfficialRating,
			Overview:        it.Overview,
			ChildCount:      it.ChildCount,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    res,
	})
}
