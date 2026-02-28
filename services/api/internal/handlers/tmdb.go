package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/gin-gonic/gin"
)

// TMDBHandler TMDB 处理器
type TMDBHandler struct {
	apiKey string
}

// NewTMDBHandler 创建 TMDB 处理器
func NewTMDBHandler() *TMDBHandler {
	return &TMDBHandler{
		apiKey: os.Getenv("TMDB_API_KEY"),
	}
}

// TMDBSearchResult TMDB 搜索结果
type TMDBSearchResult struct {
	ID            int     `json:"id"`
	Title         *string `json:"title"`
	Name          *string `json:"name"`
	OriginalTitle *string `json:"original_title"`
	OriginalName  *string `json:"original_name"`
	Overview      string  `json:"overview"`
	PosterPath    *string `json:"poster_path"`
	ReleaseDate   *string `json:"release_date"`
	FirstAirDate  *string `json:"first_air_date"`
	MediaType     *string `json:"media_type"`
}

// TMDBApiResponse TMDB API 响应
type TMDBApiResponse struct {
	Results      []TMDBSearchResult `json:"results"`
	TotalResults int                `json:"total_results"`
}

// UnifiedSearchResult 统一搜索结果格式
type UnifiedSearchResult struct {
	ID            int     `json:"id"`
	Title         string  `json:"title"`
	OriginalTitle string  `json:"originalTitle"`
	Overview      string  `json:"overview"`
	PosterPath    *string `json:"posterPath"`
	ReleaseDate   *string `json:"releaseDate"`
	MediaType     string  `json:"mediaType"`
}

// Search TMDB 搜索 API 代理
// GET /api/v1/tmdb/search?query=xxx&type=multi
func (h *TMDBHandler) Search(c *gin.Context) {
	// 获取查询参数
	query := c.Query("query")
	mediaType := c.DefaultQuery("type", "movie")

	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "缺少搜索关键词",
		})
		return
	}

	if h.apiKey == "" {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "TMDB API 未配置",
		})
		return
	}

	// 根据类型选择搜索端点
	endpoint := "search/multi"
	if mediaType == "movie" {
		endpoint = "search/movie"
	} else if mediaType == "tv" {
		endpoint = "search/tv"
	} else if mediaType != "multi" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "type 参数必须是 movie、tv 或 multi",
		})
		return
	}

	// 构建 TMDB API URL
	tmdbURL := fmt.Sprintf(
		"https://api.themoviedb.org/3/%s?api_key=%s&query=%s&language=zh-CN&page=1",
		endpoint,
		h.apiKey,
		url.QueryEscape(query),
	)

	// 调用 TMDB API
	resp, err := http.Get(tmdbURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "TMDB API 请求失败",
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("TMDB API 返回错误: %d", resp.StatusCode),
		})
		return
	}

	// 解析响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "读取响应失败",
		})
		return
	}

	var tmdbResp TMDBApiResponse
	if err := json.Unmarshal(body, &tmdbResp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "解析响应失败",
		})
		return
	}

	// 转换为统一格式
	results := make([]UnifiedSearchResult, 0, len(tmdbResp.Results))
	for _, item := range tmdbResp.Results {
		itemMediaType := mediaType
		if mediaType == "multi" && item.MediaType != nil {
			itemMediaType = *item.MediaType
		}
		if itemMediaType == "person" {
			continue
		}

		// 电影用 title，电视剧用 name
		title := ""
		if item.Title != nil {
			title = *item.Title
		} else if item.Name != nil {
			title = *item.Name
		}

		originalTitle := ""
		if item.OriginalTitle != nil {
			originalTitle = *item.OriginalTitle
		} else if item.OriginalName != nil {
			originalTitle = *item.OriginalName
		}

		releaseDate := item.ReleaseDate
		if releaseDate == nil {
			releaseDate = item.FirstAirDate
		}

		results = append(results, UnifiedSearchResult{
			ID:            item.ID,
			Title:         title,
			OriginalTitle: originalTitle,
			Overview:      item.Overview,
			PosterPath:    item.PosterPath,
			ReleaseDate:   releaseDate,
			MediaType:     itemMediaType,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"results": results,
		"total":   tmdbResp.TotalResults,
	})
}
