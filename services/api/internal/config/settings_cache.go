package config

import (
	"sync"
	"time"

	"github.com/konghang/ember/backend/internal/models"
)

var settingsKeyCacheTTL = 60 * time.Second

const gatewayRuntimeSettingCacheTTL = 5 * time.Second

type settingsCacheEntry struct {
	setting    models.Setting
	found      bool
	loadedAt   time.Time
	loadErr    error
	errorUntil time.Time
}

type settingsCacheCall struct {
	done       chan struct{}
	entry      settingsCacheEntry
	err        error
	generation uint64
}

type settingsCacheStore struct {
	mu          sync.RWMutex
	entries     map[string]settingsCacheEntry
	inflight    map[string]*settingsCacheCall
	generations map[string]uint64
}

var globalSettingsCacheStore = &settingsCacheStore{
	entries:     make(map[string]settingsCacheEntry),
	inflight:    make(map[string]*settingsCacheCall),
	generations: make(map[string]uint64),
}

func InvalidateCachedSetting(key string) {
	globalSettingsCacheStore.invalidate(key)
}

func resetSettingsCacheForTest() {
	globalSettingsCacheStore.mu.Lock()
	globalSettingsCacheStore.entries = make(map[string]settingsCacheEntry)
	globalSettingsCacheStore.inflight = make(map[string]*settingsCacheCall)
	globalSettingsCacheStore.generations = make(map[string]uint64)
	globalSettingsCacheStore.mu.Unlock()
}

func (s *settingsCacheStore) invalidate(key string) {
	if key == "" {
		return
	}
	s.mu.Lock()
	delete(s.entries, key)
	s.generations[key]++
	s.mu.Unlock()
}

func (s *settingsCacheStore) loadMany(keys []string, loader func([]string) (map[string]models.Setting, error)) (map[string]models.Setting, error) {
	return s.loadManyWithPolicy(keys, settingsKeyCacheTTL, false, loader)
}

// loadManyWithTTL reuses the bounded settings cache with a caller-specific
// freshness window. Gateway runtime switches use a short window and cache load
// failures for the same duration, while ordinary API reads retain the broader
// default cache and retry failures immediately.
func (s *settingsCacheStore) loadManyWithTTL(
	keys []string,
	ttl time.Duration,
	loader func([]string) (map[string]models.Setting, error),
) (map[string]models.Setting, error) {
	return s.loadManyWithPolicy(keys, ttl, true, loader)
}

func (s *settingsCacheStore) loadManyWithPolicy(
	keys []string,
	ttl time.Duration,
	cacheErrors bool,
	loader func([]string) (map[string]models.Setting, error),
) (map[string]models.Setting, error) {
	if ttl <= 0 {
		ttl = settingsKeyCacheTTL
	}
	for {
		result := make(map[string]models.Setting, len(keys))
		var cachedErr error
		stale := make([]string, 0, len(keys))
		staleGenerations := make(map[string]uint64, len(keys))
		seen := make(map[string]struct{}, len(keys))

		now := time.Now()
		s.mu.RLock()
		for _, key := range keys {
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			entry, ok := s.entries[key]
			fresh, entryErr := entry.current(now, ttl)
			if ok && fresh {
				if entry.found {
					result[key] = entry.setting
				}
				if cachedErr == nil {
					cachedErr = entryErr
				}
				continue
			}
			stale = append(stale, key)
			staleGenerations[key] = s.generations[key]
		}
		s.mu.RUnlock()
		if cachedErr != nil {
			return nil, cachedErr
		}

		switch len(stale) {
		case 0:
			return result, cachedErr
		case 1:
			entry, err := s.loadOneWithPolicy(stale[0], ttl, cacheErrors, loader)
			if err != nil {
				return nil, err
			}
			if entry.found {
				result[stale[0]] = entry.setting
			}
			return result, nil
		}

		loaded, err := loader(stale)
		if err != nil {
			if cacheErrors {
				retry := s.cacheLoadError(stale, staleGenerations, ttl, err)
				if retry {
					continue
				}
			}
			return nil, err
		}

		now = time.Now()
		retry := false
		s.mu.Lock()
		for _, key := range stale {
			if s.generations[key] != staleGenerations[key] {
				retry = true
				continue
			}
			entry := settingsCacheEntry{loadedAt: now}
			if setting, ok := loaded[key]; ok {
				entry.setting = setting
				entry.found = true
				result[key] = setting
			}
			s.entries[key] = entry
		}
		s.mu.Unlock()

		if retry {
			continue
		}
		return result, nil
	}
}

func (s *settingsCacheStore) loadOne(key string, loader func([]string) (map[string]models.Setting, error)) (settingsCacheEntry, error) {
	return s.loadOneWithPolicy(key, settingsKeyCacheTTL, false, loader)
}

func (s *settingsCacheStore) loadOneWithTTL(
	key string,
	ttl time.Duration,
	loader func([]string) (map[string]models.Setting, error),
) (settingsCacheEntry, error) {
	return s.loadOneWithPolicy(key, ttl, true, loader)
}

func (s *settingsCacheStore) loadOneWithPolicy(
	key string,
	ttl time.Duration,
	cacheErrors bool,
	loader func([]string) (map[string]models.Setting, error),
) (settingsCacheEntry, error) {
	if ttl <= 0 {
		ttl = settingsKeyCacheTTL
	}
	for {
		s.mu.RLock()
		if entry, ok := s.entries[key]; ok {
			fresh, err := entry.current(time.Now(), ttl)
			if fresh {
				s.mu.RUnlock()
				return entry, err
			}
		}
		s.mu.RUnlock()

		s.mu.Lock()
		staleEntry, staleEntryExists := s.entries[key]
		if staleEntryExists {
			fresh, err := staleEntry.current(time.Now(), ttl)
			if fresh {
				s.mu.Unlock()
				return staleEntry, err
			}
		}
		if call, ok := s.inflight[key]; ok {
			s.mu.Unlock()
			<-call.done

			s.mu.RLock()
			currentGeneration := s.generations[key]
			s.mu.RUnlock()
			if currentGeneration != call.generation {
				continue
			}
			return call.entry, call.err
		}
		call := &settingsCacheCall{
			done:       make(chan struct{}),
			generation: s.generations[key],
		}
		s.inflight[key] = call
		s.mu.Unlock()

		loaded, err := loader([]string{key})
		entry := settingsCacheEntry{}
		if err == nil {
			entry.loadedAt = time.Now()
			if setting, ok := loaded[key]; ok {
				entry.setting = setting
				entry.found = true
			}
		} else if cacheErrors {
			if staleEntryExists {
				entry = staleEntry
			}
			entry.loadErr = err
			entry.errorUntil = time.Now().Add(ttl)
		}

		s.mu.Lock()
		currentGeneration := s.generations[key]
		if (err == nil || cacheErrors) && currentGeneration == call.generation {
			s.entries[key] = entry
		}
		delete(s.inflight, key)
		s.mu.Unlock()

		call.entry = entry
		call.err = err
		close(call.done)

		if currentGeneration != call.generation {
			continue
		}
		return entry, err
	}
}

func (entry settingsCacheEntry) current(now time.Time, ttl time.Duration) (bool, error) {
	if entry.loadErr != nil && now.Before(entry.errorUntil) {
		return true, entry.loadErr
	}
	return !entry.loadedAt.IsZero() && now.Sub(entry.loadedAt) < ttl, nil
}

func (s *settingsCacheStore) cacheLoadError(
	keys []string,
	generations map[string]uint64,
	ttl time.Duration,
	err error,
) bool {
	now := time.Now()
	retry := false
	s.mu.Lock()
	for _, key := range keys {
		if s.generations[key] != generations[key] {
			retry = true
			continue
		}
		entry := s.entries[key]
		entry.loadErr = err
		entry.errorUntil = now.Add(ttl)
		s.entries[key] = entry
	}
	s.mu.Unlock()
	return retry
}
