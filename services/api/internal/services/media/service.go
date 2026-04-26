package media

import (
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	configpkg "github.com/konghang/ember/backend/internal/config"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
)

// latestCacheEntry 最近入库缓存条目
type latestCacheEntry struct {
	items     []embyint.EmbyItem
	timestamp time.Time
	ownerID   string // 首次请求该 key 时的 EmbyUserID；后续刷新沿用
}

// mediaLatestSFCall 用于合并并发的同 key 缓存刷新请求（singleflight 语义）
type mediaLatestSFCall struct {
	done chan struct{}
	val  []embyint.EmbyItem
	err  error
}

// MediaService 媒体服务
type MediaService struct {
	embyService    *embyint.EmbyService
	cacheMutex     sync.RWMutex
	cachedStats    *embyint.MediaStats
	cacheTimestamp time.Time
	cacheDuration  time.Duration
	statsSFMu      sync.Mutex
	statsInflight  *mediaStatsSFCall

	// 最近入库缓存
	//
	// LATEST_CACHE_PER_USER=true（默认）：key = embyUserID + ":" + itemType
	//   适用于将来开启 Emby 用户级可见性差异的场景，按用户隔离缓存，防止越权内容串联。
	// LATEST_CACHE_PER_USER=false：key = itemType（跨用户共享）
	//   适用于所有用户可见媒体库内容一致的简单场景，节省 Emby 请求次数。
	//
	// 配置变更须重启服务。
	latestMu            sync.RWMutex
	latestSFMu          sync.Mutex
	latestInflight      map[string]*mediaLatestSFCall
	cachedLatest        map[string]*latestCacheEntry
	latestCacheDuration time.Duration
	latestCacheLimit    int
}

type mediaStatsSFCall struct {
	done chan struct{}
	val  *embyint.MediaStats
	err  error
}

// NewMediaService 创建媒体服务
func NewMediaService() *MediaService {
	return &MediaService{
		embyService:         embyint.GetSharedService(),
		cacheDuration:       5 * time.Minute,  // stats 缓存 5 分钟
		latestCacheDuration: 10 * time.Minute, // 最近入库缓存 10 分钟
		latestCacheLimit:    50,               // 缓存刷新时拉取的最大条数
		cachedLatest:        make(map[string]*latestCacheEntry),
		latestInflight:      make(map[string]*mediaLatestSFCall),
	}
}

// latestCachePerUser 是否按用户分桶缓存最近入库（默认 true）
func latestCachePerUser() bool {
	configService := configpkg.NewConfigService()
	raw := strings.TrimSpace(configService.GetString("LATEST_CACHE_PER_USER"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("LATEST_CACHE_PER_USER"))
	}
	if raw == "" {
		return true // 默认按用户分桶
	}
	return strings.EqualFold(raw, "true") || raw == "1"
}

// latestCacheKey 根据 LATEST_CACHE_PER_USER 决定缓存 key
func latestCacheKey(embyUserID, itemType string) string {
	if latestCachePerUser() {
		return embyUserID + ":" + itemType
	}
	return itemType
}

// GetEmbyConfig 获取 Emby 服务器配置
func (s *MediaService) GetEmbyConfig() (string, error) {
	configService := configpkg.NewConfigService()
	url := configService.GetString("NEXT_PUBLIC_EMBY_URL")
	if url == "" {
		url = configService.GetString("EMBY_URL")
	}

	if url == "" {
		return "", nil
	}

	return url, nil
}

// GetMediaStats 获取媒体库统计信息（带 singleflight 缓存）
func (s *MediaService) GetMediaStats() (*embyint.MediaStats, error) {
	now := time.Now()
	s.cacheMutex.RLock()
	if s.cachedStats != nil && now.Sub(s.cacheTimestamp) < s.cacheDuration {
		stats := *s.cachedStats
		s.cacheMutex.RUnlock()
		return &stats, nil
	}
	s.cacheMutex.RUnlock()

	// singleflight：并发请求合并为一次 Emby 调用
	s.statsSFMu.Lock()
	if s.statsInflight != nil {
		c := s.statsInflight
		s.statsSFMu.Unlock()
		<-c.done
		return c.val, c.err
	}
	c := &mediaStatsSFCall{done: make(chan struct{})}
	s.statsInflight = c
	s.statsSFMu.Unlock()

	stats, err := s.embyService.GetMediaStats()
	if err == nil {
		s.cacheMutex.Lock()
		s.cachedStats = stats
		s.cacheTimestamp = now
		s.cacheMutex.Unlock()
	}
	c.val = stats
	c.err = err
	close(c.done)

	s.statsSFMu.Lock()
	s.statsInflight = nil
	s.statsSFMu.Unlock()

	return stats, err
}

// GetLatestItems 获取最近入库项（带 singleflight + 分桶缓存）
func (s *MediaService) GetLatestItems(embyUserID string, itemType string, limit int) ([]embyint.EmbyItem, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	cacheKey := latestCacheKey(embyUserID, itemType)
	now := time.Now()

	s.latestMu.RLock()
	entry, ok := s.cachedLatest[cacheKey]
	if ok && entry != nil && now.Sub(entry.timestamp) < s.latestCacheDuration && len(entry.items) > 0 {
		cached := entry.items
		s.latestMu.RUnlock()
		n := limit
		if n > len(cached) {
			n = len(cached)
		}
		out := make([]embyint.EmbyItem, n)
		copy(out, cached[:n])
		return out, nil
	}
	s.latestMu.RUnlock()

	// singleflight：同 key 并发请求合并为一次 Emby 调用
	s.latestSFMu.Lock()
	if inflight, found := s.latestInflight[cacheKey]; found {
		s.latestSFMu.Unlock()
		<-inflight.done
		items := inflight.val
		n := limit
		if n > len(items) {
			n = len(items)
		}
		out := make([]embyint.EmbyItem, n)
		copy(out, items[:n])
		return out, inflight.err
	}
	call := &mediaLatestSFCall{done: make(chan struct{})}
	s.latestInflight[cacheKey] = call
	s.latestSFMu.Unlock()

	// 决定刷新时用哪个 ownerID（LATEST_CACHE_PER_USER=false 时沿用首次请求的 ownerID）
	refreshOwnerID := embyUserID
	s.latestMu.RLock()
	if ok && entry != nil && entry.ownerID != "" {
		refreshOwnerID = entry.ownerID
	}
	s.latestMu.RUnlock()

	// 固定拉取 latestCacheLimit 条，再按需截断，避免"按 limit 缓存"导致的特殊情况
	items, err := s.embyService.GetLatestItems(refreshOwnerID, itemType, s.latestCacheLimit)
	if err == nil {
		items = dedupeLatestItems(items)
		ownerID := refreshOwnerID
		s.latestMu.Lock()
		if ok && entry != nil && entry.ownerID != "" {
			ownerID = entry.ownerID
		}
		s.cachedLatest[cacheKey] = &latestCacheEntry{
			items:     items,
			timestamp: now,
			ownerID:   ownerID,
		}
		s.latestMu.Unlock()
	}

	call.val = items
	call.err = err
	close(call.done)

	s.latestSFMu.Lock()
	delete(s.latestInflight, cacheKey)
	s.latestSFMu.Unlock()

	if err != nil {
		return nil, err
	}

	n := limit
	if n > len(items) {
		n = len(items)
	}
	out := make([]embyint.EmbyItem, n)
	copy(out, items[:n])
	return out, nil
}

// dedupeLatestItems 按 ID 或 (Type+Name+Year) 去重。
// ID 非空时优先使用 ID；否则按 Type+Name+Year 合并，减少同名不同版本资源被错误去重的概率
// （EmbyItem 结构暂无 ParentID，如将来增加可纳入 key 进一步精确）。
func dedupeLatestItems(items []embyint.EmbyItem) []embyint.EmbyItem {
	if len(items) <= 1 {
		return items
	}

	seen := make(map[string]struct{}, len(items))
	result := make([]embyint.EmbyItem, 0, len(items))
	for _, item := range items {
		var key string
		if item.ID != "" {
			key = item.ID
		} else {
			key = item.Type + "|" + item.Name + "|" + strconv.Itoa(item.ProductionYear)
		}
		if _, exists := seen[key]; exists {
			log.Printf("[Media] 去重最近入库重复项: type=%s key=%s name=%q id=%s", item.Type, key, item.Name, item.ID)
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}

	return result
}
