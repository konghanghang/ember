package handlers

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/common/httpx"
	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	mediapkg "github.com/konghang/ember/backend/internal/services/media"
)

// MediaHandler 媒体处理器
type MediaHandler struct {
	service *mediapkg.MediaService
	// isEmbyConfigured 用于轻量测试注入；生产路径默认指向 service.IsEmbyConfigured。
	isEmbyConfigured func() bool
}

// NewMediaHandler 创建媒体处理器
func NewMediaHandler() *MediaHandler {
	service := mediapkg.NewMediaService()
	return &MediaHandler{
		service:          service,
		isEmbyConfigured: service.IsEmbyConfigured,
	}
}

// GetEmbyConfig 获取 Emby 配置
// GET /api/v1/emby/config
//
// 协议：
//   - 系统未配置 → 200 {success:true, configured:false, url:""}
//   - 系统已配置 → 200 {success:true, configured:true, url:"..."}
//   - 真实异常   → 500 {error:"上游服务暂不可用"}
//
// "configured=false" 是合法的初始化空状态而非错误，前端据此渲染引导卡片。
func (h *MediaHandler) GetEmbyConfig(c *gin.Context) {
	if !h.isEmbyConfigured() {
		log.Printf("[Media] GetEmbyConfig: emby 未配置，返回未初始化态")
		c.JSON(http.StatusOK, gin.H{
			"success":    true,
			"configured": false,
			"url":        "",
		})
		return
	}

	url, err := h.service.GetEmbyConfig()
	if err != nil {
		httpx.InternalError(c, err)
		return
	}
	if url == "" {
		// 防御：IsConfigured 与 GetEmbyConfig 取值不一致，按未配置态返回。
		log.Printf("[Media] GetEmbyConfig: IsConfigured=true 但 url 为空，按未初始化态返回")
		c.JSON(http.StatusOK, gin.H{
			"success":    true,
			"configured": false,
			"url":        "",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"configured": true,
		"url":        url,
	})
}

// GetMediaStats 获取媒体库统计信息（带缓存）
// GET /api/v1/media/stats
//
// 协议：
//   - 系统未配置 → 200 {success:true, configured:false, data:null}
//   - 已配置成功 → 200 {success:true, configured:true, data:{...}}
//   - 已配置失败 → 500 {error:"上游服务暂不可用"}
func (h *MediaHandler) GetMediaStats(c *gin.Context) {
	if !h.isEmbyConfigured() {
		log.Printf("[Media] GetMediaStats: emby 未配置，返回未初始化态")
		c.JSON(http.StatusOK, gin.H{
			"success":    true,
			"configured": false,
			"data":       nil,
		})
		return
	}

	stats, err := h.service.GetMediaStats()
	if err != nil {
		httpx.InternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"configured": true,
		"data":       stats,
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
//
// 协议：
//   - 系统未配置        → 200 {success:true, configured:false, bound:false, data:[]}
//   - 已配置但用户未绑   → 200 {success:true, configured:true, bound:false, data:[]}
//   - 已配置已绑成功     → 200 {success:true, configured:true, bound:true, data:[...]}
//   - 已配置已绑调用失败 → 500 {error:"上游服务暂不可用"}
func (h *MediaHandler) GetLatestItems(c *gin.Context) {
	if !h.isEmbyConfigured() {
		log.Printf("[Media] GetLatestItems: emby 未配置，返回未初始化态")
		c.JSON(http.StatusOK, gin.H{
			"success":    true,
			"configured": false,
			"bound":      false,
			"data":       []LatestMediaItem{},
		})
		return
	}

	user, ok := getCurrentUserSoft(c)
	if !ok {
		return
	}

	if user.EmbyID == "" {
		log.Printf("[Media] GetLatestItems: 用户未绑定 Emby 账号，返回未初始化态")
		c.JSON(http.StatusOK, gin.H{
			"success":    true,
			"configured": true,
			"bound":      false,
			"data":       []LatestMediaItem{},
		})
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
		httpx.InternalError(c, err)
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
		"success":    true,
		"configured": true,
		"bound":      true,
		"data":       res,
	})
}

// GetPoster 获取最近入库条目封面（用户代理）
// GET /api/v1/media/posters/:itemId?type=Movie&maxHeight=400&quality=90
func (h *MediaHandler) GetPoster(c *gin.Context) {
	user, ok := getCurrentMediaUser(c)
	if !ok {
		return
	}

	itemID := strings.TrimSpace(c.Param("itemId"))
	if itemID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "itemId 不能为空"})
		return
	}

	itemType := c.DefaultQuery("type", "Movie")
	if itemType != "Movie" && itemType != "Series" {
		itemType = "Movie"
	}

	maxHeight := parsePositiveInt(c.DefaultQuery("maxHeight", "400"), 400)
	quality := parsePositiveInt(c.DefaultQuery("quality", "90"), 90)

	if err := h.service.EnsureLatestItemVisible(user.EmbyID, itemType, itemID); err != nil {
		if errors.Is(err, mediapkg.ErrLatestItemNotVisible) {
			c.JSON(http.StatusNotFound, gin.H{"error": "封面不存在"})
			return
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "获取封面失败"})
		return
	}

	content, contentType, err := h.service.GetPoster(c.Request.Context(), itemID, maxHeight, quality)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "获取封面失败"})
		return
	}

	c.Data(http.StatusOK, contentType, content)
}

// getCurrentUserSoft 校验请求是否带认证并加载用户记录，但不强制要求 EmbyID。
// 适用于"未绑定 Emby"也属于合法状态的接口（如最近入库列表）。
// 仅在认证缺失或用户不存在时写入响应；调用方需通过返回值的 ok 判断是否继续。
func getCurrentUserSoft(c *gin.Context) (*models.User, bool) {
	userIDRaw, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "未认证"})
		return nil, false
	}

	userID, ok := userIDRaw.(string)
	if !ok || userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "未认证"})
		return nil, false
	}

	var user models.User
	if err := db.DB.Select("emby_id").Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "用户不存在"})
		return nil, false
	}
	return &user, true
}

func getCurrentMediaUser(c *gin.Context) (*models.User, bool) {
	user, ok := getCurrentUserSoft(c)
	if !ok {
		return nil, false
	}
	if user.EmbyID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "用户未绑定 Emby 账号"})
		return nil, false
	}

	return user, true
}
