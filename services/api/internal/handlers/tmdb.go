package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/common/httpx"
	"github.com/konghang/ember/backend/internal/common/tmdbcache"
	"github.com/konghang/ember/backend/internal/common/upstream"
	configpkg "github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/db"
)

const (
	tmdbBaseURL           = "https://api.themoviedb.org/3"
	tmdbSearchCacheTTL    = 30 * time.Minute
	tmdbTVDetailCacheTTL  = 24 * time.Hour
	tmdbResponseLanguage  = "zh-CN"
	tmdbProxyCacheVersion = "v1"
)

// TMDBHandler TMDB 处理器
type TMDBHandler struct {
	configService *configpkg.ConfigService
	cacheStore    *tmdbcache.Store
	httpClient    *http.Client
	baseURL       string
}

// NewTMDBHandler 创建 TMDB 处理器
func NewTMDBHandler() *TMDBHandler {
	return &TMDBHandler{
		configService: configpkg.NewConfigService(),
		cacheStore:    tmdbcache.NewStore(),
		httpClient:    http.DefaultClient,
		baseURL:       tmdbBaseURL,
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

type TMDBTVSeason struct {
	SeasonNumber int `json:"season_number"`
}

type TMDBTVDetailResponse struct {
	ID              int            `json:"id"`
	Name            string         `json:"name"`
	NumberOfSeasons int            `json:"number_of_seasons"`
	Seasons         []TMDBTVSeason `json:"seasons"`
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

type TMDBTVSeasonOptions struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	NumberOfSeasons int    `json:"numberOfSeasons"`
	Seasons         []int  `json:"seasons"`
}

func buildTMDBTVSeasonOptions(detail TMDBTVDetailResponse) TMDBTVSeasonOptions {
	seasons := make([]int, 0, len(detail.Seasons))
	seen := make(map[int]struct{}, len(detail.Seasons))
	for _, item := range detail.Seasons {
		if item.SeasonNumber <= 0 {
			continue
		}
		if _, exists := seen[item.SeasonNumber]; exists {
			continue
		}
		seen[item.SeasonNumber] = struct{}{}
		seasons = append(seasons, item.SeasonNumber)
	}

	if len(seasons) == 0 && detail.NumberOfSeasons > 0 {
		seasons = make([]int, 0, detail.NumberOfSeasons)
		for season := 1; season <= detail.NumberOfSeasons; season++ {
			seasons = append(seasons, season)
		}
	}

	return TMDBTVSeasonOptions{
		ID:              detail.ID,
		Name:            detail.Name,
		NumberOfSeasons: detail.NumberOfSeasons,
		Seasons:         seasons,
	}
}

func buildTMDBProxyCacheKey(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return fmt.Sprintf("proxy:%s:%s", tmdbProxyCacheVersion, hex.EncodeToString(sum[:]))
}

func (h *TMDBHandler) fetchTMDB(c *gin.Context, cacheKey, endpoint string, ttl time.Duration, target interface{}) bool {
	apiKey := h.configService.GetString("TMDB_API_KEY")
	if apiKey == "" {
		httpx.InternalError(c, errors.New("tmdb api key is not configured"))
		return false
	}

	baseURL := strings.TrimRight(h.baseURL, "/")
	if baseURL == "" {
		baseURL = tmdbBaseURL
	}

	tmdbURL := fmt.Sprintf("%s/%s", baseURL, strings.TrimLeft(endpoint, "/"))
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, tmdbURL, nil)
	if err != nil {
		httpx.InternalError(c, upstream.SafeUpstreamError(err, "tmdb"))
		return false
	}

	query := req.URL.Query()
	query.Set("api_key", apiKey)
	query.Set("language", tmdbResponseLanguage)
	req.URL.RawQuery = query.Encode()

	cacheStore := h.cacheStore
	if cacheStore == nil {
		cacheStore = tmdbcache.NewStore()
	}

	httpClient := h.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	if err := cacheStore.FetchJSON(c.Request.Context(), db.DB, httpClient, cacheKey, req.URL.String(), ttl, false, target); err != nil {
		httpx.InternalError(c, err)
		return false
	}

	return true
}

// Search TMDB 搜索 API 代理
// GET /api/v1/tmdb/search?query=xxx&type=multi
func (h *TMDBHandler) Search(c *gin.Context) {
	// 获取查询参数
	query := strings.TrimSpace(c.Query("query"))
	mediaType := c.DefaultQuery("type", "movie")

	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "缺少搜索关键词",
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

	var tmdbResp TMDBApiResponse
	searchEndpoint := fmt.Sprintf("%s?query=%s&page=1", endpoint, url.QueryEscape(query))
	cacheKey := buildTMDBProxyCacheKey("search", mediaType, tmdbResponseLanguage, query)
	if !h.fetchTMDB(c, cacheKey, searchEndpoint, tmdbSearchCacheTTL, &tmdbResp) {
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

// GetTVSeasons TMDB 剧集季列表 API 代理
// GET /api/v1/tmdb/tv/:id/seasons
func (h *TMDBHandler) GetTVSeasons(c *gin.Context) {
	tvID := c.Param("id")
	if tvID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "缺少剧集 ID",
		})
		return
	}

	var detail TMDBTVDetailResponse
	cacheKey := buildTMDBProxyCacheKey("tv-detail", tmdbResponseLanguage, tvID)
	if !h.fetchTMDB(c, cacheKey, fmt.Sprintf("tv/%s", url.PathEscape(tvID)), tmdbTVDetailCacheTTL, &detail) {
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": buildTMDBTVSeasonOptions(detail),
	})
}
