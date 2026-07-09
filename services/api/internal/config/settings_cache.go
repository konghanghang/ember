package config

import (
	"sync"
	"time"

	"github.com/konghang/ember/backend/internal/models"
)

var settingsKeyCacheTTL = 60 * time.Second

type settingsCacheEntry struct {
	setting  models.Setting
	found    bool
	loadedAt time.Time
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
	for {
		result := make(map[string]models.Setting, len(keys))
		stale := make([]string, 0, len(keys))
		staleGenerations := make(map[string]uint64, len(keys))
		seen := make(map[string]struct{}, len(keys))

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
			if ok && time.Since(entry.loadedAt) < settingsKeyCacheTTL {
				if entry.found {
					result[key] = entry.setting
				}
				continue
			}
			stale = append(stale, key)
			staleGenerations[key] = s.generations[key]
		}
		s.mu.RUnlock()

		switch len(stale) {
		case 0:
			return result, nil
		case 1:
			entry, err := s.loadOne(stale[0], loader)
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
			return nil, err
		}

		now := time.Now()
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
	for {
		s.mu.RLock()
		if entry, ok := s.entries[key]; ok && time.Since(entry.loadedAt) < settingsKeyCacheTTL {
			s.mu.RUnlock()
			return entry, nil
		}
		s.mu.RUnlock()

		s.mu.Lock()
		if entry, ok := s.entries[key]; ok && time.Since(entry.loadedAt) < settingsKeyCacheTTL {
			s.mu.Unlock()
			return entry, nil
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
		entry := settingsCacheEntry{loadedAt: time.Now()}
		if err == nil {
			if setting, ok := loaded[key]; ok {
				entry.setting = setting
				entry.found = true
			}
		}

		s.mu.Lock()
		currentGeneration := s.generations[key]
		if err == nil && currentGeneration == call.generation {
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
