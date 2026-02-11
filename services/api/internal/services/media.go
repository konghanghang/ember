package services

import (
	"os"
	"sync"
	"time"
)

// MediaService 媒体服务
type MediaService struct {
	embyService    *EmbyService
	cacheMutex     sync.RWMutex
	cachedStats    *MediaStats
	cacheTimestamp time.Time
	cacheDuration  time.Duration
}

// NewMediaService 创建媒体服务
func NewMediaService() *MediaService {
	return &MediaService{
		embyService:   NewEmbyService(),
		cacheDuration: 5 * time.Minute, // 缓存 5 分钟
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
