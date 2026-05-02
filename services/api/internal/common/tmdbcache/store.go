package tmdbcache

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/konghang/ember/backend/internal/common/upstream"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type memoryEntry struct {
	Payload   []byte
	ExpiresAt time.Time
}

type fetchCall struct {
	done    chan struct{}
	payload []byte
	err     error
}

type Store struct {
	mu       sync.RWMutex
	memory   map[string]memoryEntry
	fetchMu  sync.Mutex
	inflight map[string]*fetchCall
	now      func() time.Time
}

func NewStore() *Store {
	return &Store{
		memory:   make(map[string]memoryEntry),
		inflight: make(map[string]*fetchCall),
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (s *Store) FetchJSON(
	ctx context.Context,
	dbConn *gorm.DB,
	httpClient *http.Client,
	cacheKey string,
	requestURL string,
	ttl time.Duration,
	force bool,
	out interface{},
) error {
	now := s.now()
	if !force {
		if payload, ok := s.getMemory(cacheKey, now); ok {
			if err := json.Unmarshal(payload, out); err == nil {
				return nil
			}
		}

		if dbConn != nil {
			var cached models.TMDBCache
			err := dbConn.Where("\"cache_key\" = ? AND \"expires_at\" > ?", cacheKey, now).First(&cached).Error
			if err == nil {
				payload := []byte(cached.CacheValue)
				if decodeErr := json.Unmarshal(payload, out); decodeErr == nil {
					s.setMemory(cacheKey, payload, cached.ExpiresAt)
					return nil
				}
			}
		}
	}

	call, leader := s.beginFetch(cacheKey)
	if !leader {
		<-call.done
		if call.err != nil {
			return call.err
		}
		if err := json.Unmarshal(call.payload, out); err != nil {
			return fmt.Errorf("解析 TMDB 响应失败: %w", err)
		}
		return nil
	}
	defer s.finishFetch(cacheKey, call)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		call.err = upstream.SafeUpstreamError(err, "tmdb")
		return call.err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		call.err = upstream.SafeUpstreamError(err, "tmdb")
		return call.err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		call.err = upstream.SafeUpstreamError(err, "tmdb")
		return call.err
	}

	if resp.StatusCode != http.StatusOK {
		call.err = upstream.SafeUpstreamHTTPError("tmdb", resp.StatusCode)
		return call.err
	}

	if err := json.Unmarshal(body, out); err != nil {
		call.err = fmt.Errorf("解析 TMDB 响应失败: %w", err)
		return call.err
	}

	expiresAt := now.Add(ttl)
	s.setMemory(cacheKey, body, expiresAt)

	if dbConn != nil {
		cacheRow := models.TMDBCache{
			CacheKey:   cacheKey,
			CacheValue: string(body),
			ExpiresAt:  expiresAt,
		}
		if err := dbConn.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "cache_key"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"cache_value": cacheRow.CacheValue,
				"expires_at":  cacheRow.ExpiresAt,
			}),
		}).Create(&cacheRow).Error; err != nil {
			call.err = fmt.Errorf("写入 TMDB 缓存失败: %w", err)
			return call.err
		}
	}

	call.payload = append([]byte(nil), body...)
	return nil
}

func (s *Store) getMemory(cacheKey string, now time.Time) ([]byte, bool) {
	s.mu.RLock()
	entry, ok := s.memory[cacheKey]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if now.After(entry.ExpiresAt) {
		s.mu.Lock()
		if current, exists := s.memory[cacheKey]; exists && now.After(current.ExpiresAt) {
			delete(s.memory, cacheKey)
		}
		s.mu.Unlock()
		return nil, false
	}
	return entry.Payload, true
}

func (s *Store) setMemory(cacheKey string, payload []byte, expiresAt time.Time) {
	copyPayload := make([]byte, len(payload))
	copy(copyPayload, payload)
	s.mu.Lock()
	s.memory[cacheKey] = memoryEntry{Payload: copyPayload, ExpiresAt: expiresAt}
	s.mu.Unlock()
}

func (s *Store) beginFetch(cacheKey string) (*fetchCall, bool) {
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()

	if existing, ok := s.inflight[cacheKey]; ok {
		return existing, false
	}
	call := &fetchCall{done: make(chan struct{})}
	s.inflight[cacheKey] = call
	return call, true
}

func (s *Store) finishFetch(cacheKey string, call *fetchCall) {
	s.fetchMu.Lock()
	delete(s.inflight, cacheKey)
	s.fetchMu.Unlock()
	close(call.done)
}
