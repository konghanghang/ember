package services

import (
	"os"
	"sync"
	"time"
)

type latestCacheEntry struct {
	items     []EmbyItem
	timestamp time.Time
	ownerID   string // 首次请求该 itemType 时的 EmbyUserID；后续刷新沿用
}

// MediaService 媒体服务
type MediaService struct {
	embyService    *EmbyService
	cacheMutex     sync.RWMutex
	cachedStats    *MediaStats
	cacheTimestamp time.Time
	cacheDuration  time.Duration

	// 最近入库缓存（IMPORTANT）
	//
	// 这里故意按 itemType（Movie/Series）做“跨用户共享缓存”，并且刷新时沿用第一次请求的 ownerID。
	// 这个实现依赖一个前提：所有普通用户在 Emby 侧看到的媒体库内容一致（没有用户级可见性差异）。
	//
	// 如果未来开启了 Emby 的用户级权限差异（家长控制、库权限、标签过滤等），必须把缓存隔离到用户维度：
	// 方案 A：key = embyUserID + itemType
	// 方案 B：完全不缓存 Latest（或仅做 very short TTL）
	//
	// 否则会出现内容串联，严重时属于越权信息暴露。
	latestMutex         sync.RWMutex
	cachedLatest        map[string]*latestCacheEntry // key: Movie / Series
	latestCacheDuration time.Duration
	latestCacheLimit    int
}

// NewMediaService 创建媒体服务
func NewMediaService() *MediaService {
	return &MediaService{
		embyService:         NewEmbyService(),
		cacheDuration:       5 * time.Minute,  // stats 缓存 5 分钟
		latestCacheDuration: 10 * time.Minute, // 最近入库缓存 10 分钟
		latestCacheLimit:    50,               // 缓存刷新时拉取的最大条数
		cachedLatest:        make(map[string]*latestCacheEntry),
	}
}

// GetEmbyConfig 获取 Emby 服务器配置
func (s *MediaService) GetEmbyConfig() (string, error) {
	// 优先返回公网地址，回退到内部地址
	url := os.Getenv("NEXT_PUBLIC_EMBY_URL")
	if url == "" {
		url = os.Getenv("EMBY_URL")
	}

	if url == "" {
		return "", nil
	}

	return url, nil
}

// GetMediaStats 获取媒体库统计信息（带缓存）
func (s *MediaService) GetMediaStats() (*MediaStats, error) {
	// 检查缓存
	s.cacheMutex.RLock()
	now := time.Now()
	if s.cachedStats != nil && now.Sub(s.cacheTimestamp) < s.cacheDuration {
		stats := *s.cachedStats
		s.cacheMutex.RUnlock()
		return &stats, nil
	}
	s.cacheMutex.RUnlock()

	// 缓存过期，重新获取
	stats, err := s.embyService.GetMediaStats()
	if err != nil {
		return nil, err
	}

	// 更新缓存
	s.cacheMutex.Lock()
	s.cachedStats = stats
	s.cacheTimestamp = now
	s.cacheMutex.Unlock()

	return stats, nil
}

// GetLatestItems 获取最近入库项（带缓存）
func (s *MediaService) GetLatestItems(embyUserID string, itemType string, limit int) ([]EmbyItem, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	now := time.Now()
	s.latestMutex.RLock()
	entry, ok := s.cachedLatest[itemType]
	if ok && entry != nil && now.Sub(entry.timestamp) < s.latestCacheDuration && len(entry.items) > 0 {
		cached := entry.items
		s.latestMutex.RUnlock()
		n := limit
		if n > len(cached) {
			n = len(cached)
		}
		out := make([]EmbyItem, n)
		copy(out, cached[:n])
		return out, nil
	}
	s.latestMutex.RUnlock()

	refreshOwnerID := embyUserID
	if ok && entry != nil && entry.ownerID != "" {
		refreshOwnerID = entry.ownerID
	}

	// 缓存过期，刷新：固定拉取 50 条，再按需截断返回，避免“按 limit 缓存”带来的特殊情况。
	items, err := s.embyService.GetLatestItems(refreshOwnerID, itemType, s.latestCacheLimit)
	if err != nil {
		return nil, err
	}

	s.latestMutex.Lock()
	ownerID := refreshOwnerID
	if ok && entry != nil && entry.ownerID != "" {
		ownerID = entry.ownerID
	}
	s.cachedLatest[itemType] = &latestCacheEntry{
		items:     items,
		timestamp: now,
		ownerID:   ownerID,
	}
	s.latestMutex.Unlock()

	n := limit
	if n > len(items) {
		n = len(items)
	}
	out := make([]EmbyItem, n)
	copy(out, items[:n])
	return out, nil
}
