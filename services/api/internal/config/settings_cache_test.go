package config

import (
	"sync"
	"testing"

	"github.com/konghang/ember/backend/internal/models"
)

func TestResolveStringUsesKeyCacheForRepeatedDatabaseReads(t *testing.T) {
	resetSettingsCacheForTest()
	t.Cleanup(resetSettingsCacheForTest)

	definitions := getConfigDefinitionMap()
	original, existed := definitions["TEST_CACHE_KEY"]
	definitions["TEST_CACHE_KEY"] = ConfigDefinition{
		Key:                "TEST_CACHE_KEY",
		Type:               ConfigValueString,
		DisableEnvFallback: true,
	}
	t.Cleanup(func() {
		if existed {
			definitions["TEST_CACHE_KEY"] = original
			return
		}
		delete(definitions, "TEST_CACHE_KEY")
	})

	loadCalls := 0
	service := &ConfigService{
		loadSettingRecords: func(keys []string) (map[string]models.Setting, error) {
			loadCalls++
			return map[string]models.Setting{
				"TEST_CACHE_KEY": {
					Key:   "TEST_CACHE_KEY",
					Value: "cached-value",
				},
			}, nil
		},
	}

	first, source, err := service.ResolveString("TEST_CACHE_KEY")
	if err != nil {
		t.Fatalf("first ResolveString failed: %v", err)
	}
	second, secondSource, err := service.ResolveString("TEST_CACHE_KEY")
	if err != nil {
		t.Fatalf("second ResolveString failed: %v", err)
	}

	if first != "cached-value" || second != "cached-value" {
		t.Fatalf("unexpected cached values: first=%q second=%q", first, second)
	}
	if source != ConfigSourceDatabase || secondSource != ConfigSourceDatabase {
		t.Fatalf("expected database source for both reads, got %s / %s", source, secondSource)
	}
	if loadCalls != 1 {
		t.Fatalf("expected database loader to be called once, got %d", loadCalls)
	}
}

func TestResolveStringCachesMissingDatabaseKey(t *testing.T) {
	resetSettingsCacheForTest()
	t.Cleanup(resetSettingsCacheForTest)

	definitions := getConfigDefinitionMap()
	original, existed := definitions["TEST_MISSING_CACHE_KEY"]
	definitions["TEST_MISSING_CACHE_KEY"] = ConfigDefinition{
		Key:                "TEST_MISSING_CACHE_KEY",
		Type:               ConfigValueString,
		DisableEnvFallback: true,
	}
	t.Cleanup(func() {
		if existed {
			definitions["TEST_MISSING_CACHE_KEY"] = original
			return
		}
		delete(definitions, "TEST_MISSING_CACHE_KEY")
	})

	loadCalls := 0
	service := &ConfigService{
		loadSettingRecords: func(keys []string) (map[string]models.Setting, error) {
			loadCalls++
			return map[string]models.Setting{}, nil
		},
	}

	first, source, err := service.ResolveString("TEST_MISSING_CACHE_KEY")
	if err != nil {
		t.Fatalf("first ResolveString failed: %v", err)
	}
	second, secondSource, err := service.ResolveString("TEST_MISSING_CACHE_KEY")
	if err != nil {
		t.Fatalf("second ResolveString failed: %v", err)
	}

	if first != "" || second != "" {
		t.Fatalf("expected missing value to resolve empty, got %q / %q", first, second)
	}
	if source != ConfigSourceUnset || secondSource != ConfigSourceUnset {
		t.Fatalf("expected unset source for missing key, got %s / %s", source, secondSource)
	}
	if loadCalls != 1 {
		t.Fatalf("expected missing key to be negative-cached after one DB read, got %d", loadCalls)
	}
}

func TestInvalidateCachedSettingForcesReload(t *testing.T) {
	resetSettingsCacheForTest()
	t.Cleanup(resetSettingsCacheForTest)

	definitions := getConfigDefinitionMap()
	original, existed := definitions["TEST_INVALIDATE_CACHE_KEY"]
	definitions["TEST_INVALIDATE_CACHE_KEY"] = ConfigDefinition{
		Key:                "TEST_INVALIDATE_CACHE_KEY",
		Type:               ConfigValueString,
		DisableEnvFallback: true,
	}
	t.Cleanup(func() {
		if existed {
			definitions["TEST_INVALIDATE_CACHE_KEY"] = original
			return
		}
		delete(definitions, "TEST_INVALIDATE_CACHE_KEY")
	})

	loadCalls := 0
	currentValue := "value-v1"
	service := &ConfigService{
		loadSettingRecords: func(keys []string) (map[string]models.Setting, error) {
			loadCalls++
			return map[string]models.Setting{
				"TEST_INVALIDATE_CACHE_KEY": {
					Key:   "TEST_INVALIDATE_CACHE_KEY",
					Value: currentValue,
				},
			}, nil
		},
	}

	value, _, err := service.ResolveString("TEST_INVALIDATE_CACHE_KEY")
	if err != nil {
		t.Fatalf("first ResolveString failed: %v", err)
	}
	if value != "value-v1" {
		t.Fatalf("unexpected first value: %q", value)
	}

	currentValue = "value-v2"
	InvalidateCachedSetting("TEST_INVALIDATE_CACHE_KEY")

	value, _, err = service.ResolveString("TEST_INVALIDATE_CACHE_KEY")
	if err != nil {
		t.Fatalf("second ResolveString failed: %v", err)
	}
	if value != "value-v2" {
		t.Fatalf("expected cache invalidation to reload new value, got %q", value)
	}
	if loadCalls != 2 {
		t.Fatalf("expected invalidation to force second DB read, got %d", loadCalls)
	}
}

func TestResolveStringCoalescesConcurrentSingleKeyLoads(t *testing.T) {
	resetSettingsCacheForTest()
	t.Cleanup(resetSettingsCacheForTest)

	definitions := getConfigDefinitionMap()
	original, existed := definitions["TEST_CONCURRENT_CACHE_KEY"]
	definitions["TEST_CONCURRENT_CACHE_KEY"] = ConfigDefinition{
		Key:                "TEST_CONCURRENT_CACHE_KEY",
		Type:               ConfigValueString,
		DisableEnvFallback: true,
	}
	t.Cleanup(func() {
		if existed {
			definitions["TEST_CONCURRENT_CACHE_KEY"] = original
			return
		}
		delete(definitions, "TEST_CONCURRENT_CACHE_KEY")
	})

	var mu sync.Mutex
	loadCalls := 0
	release := make(chan struct{})
	service := &ConfigService{
		loadSettingRecords: func(keys []string) (map[string]models.Setting, error) {
			mu.Lock()
			loadCalls++
			mu.Unlock()
			<-release
			return map[string]models.Setting{
				"TEST_CONCURRENT_CACHE_KEY": {
					Key:   "TEST_CONCURRENT_CACHE_KEY",
					Value: "shared-value",
				},
			}, nil
		},
	}

	const goroutineCount = 8
	var wg sync.WaitGroup
	results := make([]string, goroutineCount)
	sources := make([]string, goroutineCount)
	errorsSeen := make([]error, goroutineCount)

	wg.Add(goroutineCount)
	for i := 0; i < goroutineCount; i++ {
		go func(index int) {
			defer wg.Done()
			value, source, err := service.ResolveString("TEST_CONCURRENT_CACHE_KEY")
			results[index] = value
			sources[index] = source
			errorsSeen[index] = err
		}(i)
	}

	close(release)
	wg.Wait()

	for i := 0; i < goroutineCount; i++ {
		if errorsSeen[i] != nil {
			t.Fatalf("goroutine %d ResolveString failed: %v", i, errorsSeen[i])
		}
		if results[i] != "shared-value" {
			t.Fatalf("goroutine %d expected shared-value, got %q", i, results[i])
		}
		if sources[i] != ConfigSourceDatabase {
			t.Fatalf("goroutine %d expected database source, got %s", i, sources[i])
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if loadCalls != 1 {
		t.Fatalf("expected concurrent reads to share one loader call, got %d", loadCalls)
	}
}
