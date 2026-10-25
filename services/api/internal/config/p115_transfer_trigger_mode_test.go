package config

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbpkg "github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestP115TransferTriggerModeDefinitionAndDefault locks the administrator-facing
// enum contract and prevents environment values from enabling compatibility.
func TestP115TransferTriggerModeDefinitionAndDefault(t *testing.T) {
	resetSettingsCacheForTest()
	t.Cleanup(resetSettingsCacheForTest)
	t.Setenv(P115TransferTriggerModeKey, TransferTriggerModeCompatible)
	definition, ok := getConfigDefinitionMap()[P115TransferTriggerModeKey]
	if !ok {
		t.Fatal("missing transfer trigger mode definition")
	}
	if definition.Group != ConfigGroupMedia || definition.Type != ConfigValueEnum ||
		definition.DefaultValue != TransferTriggerModeStrict || !definition.Editable ||
		definition.Sensitive || definition.RestartRequired || definition.AllowEmpty ||
		definition.EnvKey != "" || !definition.DisableEnvFallback {
		t.Fatalf("unexpected transfer trigger mode definition: %+v", definition)
	}
	if definition.Label != "115 首次转存策略" || definition.Description != "兼容模式下，视频预读也可能触发转存" {
		t.Fatal("administrator metadata does not describe the compatibility tradeoff")
	}
	wantOptions := []ConfigOption{
		{Label: "严格模式", Value: TransferTriggerModeStrict},
		{Label: "兼容模式", Value: TransferTriggerModeCompatible},
	}
	if !slices.Equal(definition.Options, wantOptions) || definition.Normalize == nil || definition.Validate == nil {
		t.Fatal("transfer trigger mode must expose exactly two validated options")
	}
	loads := 0
	service := &ConfigService{loadSettingRecords: func(keys []string) (map[string]models.Setting, error) {
		loads++
		if len(keys) != 1 || keys[0] != P115TransferTriggerModeKey {
			t.Fatalf("unexpected keys: %v", keys)
		}
		return map[string]models.Setting{}, nil
	}}
	for range 2 {
		mode, err := service.P115TransferTriggerMode(context.Background())
		if err != nil || mode != TransferTriggerModeStrict {
			t.Fatalf("missing setting mode=%q error=%v, want strict despite environment", mode, err)
		}
	}
	item, err := service.Get(P115TransferTriggerModeKey)
	if err != nil || item.Value == nil || *item.Value != TransferTriggerModeStrict || item.Source != ConfigSourceDefault || loads != 1 {
		t.Fatalf("default item=%+v error=%v loads=%d, want negative cache and default source", item, err, loads)
	}
}

// TestP115TransferTriggerModeStoredValues validates persisted values separately
// from a missing setting, including an explicitly empty database value.
func TestP115TransferTriggerModeStoredValues(t *testing.T) {
	for _, test := range []struct {
		value string
		want  string
	}{
		{value: "strict", want: TransferTriggerModeStrict},
		{value: " CoMpAtIbLe \t", want: TransferTriggerModeCompatible},
		{value: " StRiCt ", want: TransferTriggerModeStrict},
		{value: ""},
		{value: " \t"},
		{value: "true"},
		{value: "fixture-private-invalid-mode"},
	} {
		t.Run(test.value, func(t *testing.T) {
			resetSettingsCacheForTest()
			t.Cleanup(resetSettingsCacheForTest)
			service := &ConfigService{loadSettingRecords: func([]string) (map[string]models.Setting, error) {
				return map[string]models.Setting{P115TransferTriggerModeKey: {Key: P115TransferTriggerModeKey, Value: test.value}}, nil
			}}
			mode, err := service.P115TransferTriggerMode(context.Background())
			if test.want != "" {
				if err != nil || mode != test.want {
					t.Fatalf("stored mode=%q error=%v, want %q", mode, err, test.want)
				}
				return
			}
			if mode != "" || !errors.Is(err, ErrConfigValidation) {
				t.Fatalf("invalid stored mode=%q error=%v, want no mode and validation error", mode, err)
			}
			if strings.Contains(err.Error(), "fixture-private-invalid-mode") {
				t.Fatal("validation error leaked the stored value")
			}
			if _, err := service.Update(P115TransferTriggerModeKey, UpdateConfigRequest{Value: &test.value}, ""); !errors.Is(err, ErrConfigValidation) {
				t.Fatalf("invalid save error=%v, want validation before database access", err)
			}
		})
	}
}

// TestP115TransferTriggerModeRuntimeCacheFailsClosed proves that only fresh
// compatible values can be returned and expired storage failures replace them.
func TestP115TransferTriggerModeRuntimeCacheFailsClosed(t *testing.T) {
	resetSettingsCacheForTest()
	t.Cleanup(resetSettingsCacheForTest)
	current := TransferTriggerModeCompatible
	loads := 0
	var loadErr error
	service := &ConfigService{loadSettingRecords: func([]string) (map[string]models.Setting, error) {
		loads++
		if loadErr != nil {
			return nil, loadErr
		}
		return map[string]models.Setting{P115TransferTriggerModeKey: {Key: P115TransferTriggerModeKey, Value: current}}, nil
	}}
	if mode, err := service.P115TransferTriggerMode(context.Background()); err != nil || mode != TransferTriggerModeCompatible {
		t.Fatalf("initial mode=%q error=%v", mode, err)
	}
	current = TransferTriggerModeStrict
	if mode, err := service.P115TransferTriggerMode(context.Background()); err != nil || mode != TransferTriggerModeCompatible || loads != 1 {
		t.Fatalf("fresh cache mode=%q error=%v loads=%d", mode, err, loads)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if mode, err := service.P115TransferTriggerMode(ctx); mode != "" || !errors.Is(err, context.Canceled) || loads != 1 {
		t.Fatalf("canceled request mode=%q error=%v loads=%d", mode, err, loads)
	}
	// Six seconds is stale for Gateway policy but still fresh for ordinary settings.
	globalSettingsCacheStore.mu.Lock()
	entry := globalSettingsCacheStore.entries[P115TransferTriggerModeKey]
	entry.loadedAt = time.Now().Add(-gatewayRuntimeSettingCacheTTL - time.Second)
	globalSettingsCacheStore.entries[P115TransferTriggerModeKey] = entry
	globalSettingsCacheStore.mu.Unlock()
	if mode, err := service.P115TransferTriggerMode(context.Background()); err != nil || mode != TransferTriggerModeStrict || loads != 2 {
		t.Fatalf("short-TTL refresh mode=%q error=%v loads=%d", mode, err, loads)
	}
	current = TransferTriggerModeCompatible
	InvalidateCachedSetting(P115TransferTriggerModeKey)
	if mode, err := service.P115TransferTriggerMode(context.Background()); err != nil || mode != TransferTriggerModeCompatible {
		t.Fatalf("replacement mode=%q error=%v", mode, err)
	}
	expireCachedSettingForTest(P115TransferTriggerModeKey)
	loadErr = errors.New("fixture storage unavailable")
	for range 2 {
		mode, err := service.P115TransferTriggerMode(context.Background())
		if mode != "" || !errors.Is(err, loadErr) || loads != 4 {
			t.Fatalf("failed refresh mode=%q error=%v loads=%d, want no stale compatibility and one backed-off load", mode, err, loads)
		}
	}
	loadErr, current = nil, TransferTriggerModeStrict
	expireCachedSettingForTest(P115TransferTriggerModeKey)
	if mode, err := service.P115TransferTriggerMode(context.Background()); err != nil || mode != TransferTriggerModeStrict || loads != 5 {
		t.Fatalf("recovered mode=%q error=%v loads=%d", mode, err, loads)
	}
}

// TestP115TransferTriggerModeCanceledRefreshAllowsHealthyRetry prevents a
// request-local termination from retaining stale compatibility or shared backoff.
func TestP115TransferTriggerModeCanceledRefreshAllowsHealthyRetry(t *testing.T) {
	for _, termination := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(termination.Error(), func(t *testing.T) {
			resetSettingsCacheForTest()
			t.Cleanup(resetSettingsCacheForTest)
			loads := 0
			service := &ConfigService{loadSettingRecords: func([]string) (map[string]models.Setting, error) {
				loads++
				if loads == 2 {
					return nil, fmt.Errorf("fixture query: %w", termination)
				}
				mode := TransferTriggerModeCompatible
				if loads > 2 {
					mode = TransferTriggerModeStrict
				}
				return map[string]models.Setting{P115TransferTriggerModeKey: {Key: P115TransferTriggerModeKey, Value: mode}}, nil
			}}
			if _, err := service.P115TransferTriggerMode(context.Background()); err != nil {
				t.Fatal(err)
			}
			expireCachedSettingForTest(P115TransferTriggerModeKey)
			if mode, err := service.P115TransferTriggerMode(context.Background()); mode != "" || !errors.Is(err, termination) {
				t.Fatalf("terminated refresh mode=%q error=%v", mode, err)
			}
			if mode, err := service.P115TransferTriggerMode(context.Background()); err != nil || mode != TransferTriggerModeStrict || loads != 3 {
				t.Fatalf("healthy retry mode=%q error=%v loads=%d", mode, err, loads)
			}
		})
	}
}

// TestP115TransferTriggerModeUpdateNormalizesAndInvalidatesCache exercises the
// real settings update boundary with fake SQL and no external database.
func TestP115TransferTriggerModeUpdateNormalizesAndInvalidatesCache(t *testing.T) {
	for _, test := range []struct {
		previous, input, want string
	}{
		{previous: TransferTriggerModeStrict, input: " CoMpAtIbLe ", want: TransferTriggerModeCompatible},
		{previous: TransferTriggerModeCompatible, input: " StRiCt ", want: TransferTriggerModeStrict},
	} {
		t.Run(test.want, func(t *testing.T) {
			resetSettingsCacheForTest()
			t.Cleanup(resetSettingsCacheForTest)
			raw, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = raw.Close() })
			database, err := gorm.Open(postgres.New(postgres.Config{Conn: raw}), &gorm.Config{
				DisableAutomaticPing: true, SkipDefaultTransaction: true, Logger: logger.Default.LogMode(logger.Silent),
			})
			if err != nil {
				t.Fatal(err)
			}
			previousDatabase := dbpkg.DB
			dbpkg.DB = database
			t.Cleanup(func() { dbpkg.DB = previousDatabase })
			mock.ExpectQuery(`SELECT .* FROM "settings" WHERE key IN`).WithArgs(P115TransferTriggerModeKey).
				WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow(P115TransferTriggerModeKey, test.previous))
			service := &ConfigService{}
			if mode, err := service.P115TransferTriggerMode(context.Background()); err != nil || mode != test.previous {
				t.Fatalf("previous mode=%q error=%v", mode, err)
			}
			mock.ExpectExec(`INSERT INTO "settings"`).WithArgs(P115TransferTriggerModeKey, test.want, false, "fixture-admin", sqlmock.AnyArg(), false, "fixture-admin", test.want).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery(`SELECT .* FROM "settings" WHERE key IN`).WithArgs(P115TransferTriggerModeKey).
				WillReturnRows(sqlmock.NewRows([]string{"key", "value"}).AddRow(P115TransferTriggerModeKey, test.want))
			item, err := service.Update(P115TransferTriggerModeKey, UpdateConfigRequest{Value: &test.input}, "fixture-admin")
			if err != nil || item.Value == nil || *item.Value != test.want || item.Source != ConfigSourceDatabase {
				t.Fatalf("updated item=%+v error=%v", item, err)
			}
			if mode, err := service.P115TransferTriggerMode(context.Background()); err != nil || mode != test.want {
				t.Fatalf("post-update mode=%q error=%v, stale policy survived cache invalidation", mode, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
