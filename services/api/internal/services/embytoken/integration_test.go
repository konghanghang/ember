package embytoken

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	dbpkg "github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const tokenIntegrationDatabaseEnv = "EMBER_INTEGRATION_DATABASE_URL"

func TestIntegrationConcurrentAuthenticationCreatesOneDigestMapping(t *testing.T) {
	database := newTokenIntegrationDatabase(t)
	seedTokenUser(t, database, models.User{
		ID: "user_token_1", Username: "token-user-1", Role: "user",
		EmbyID: "emby-user-1", IsActive: true,
	})
	service, err := NewService(database, "fixture-token-root-key", testServerID)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	const workerCount = 8
	start := make(chan struct{})
	errs := make(chan error, workerCount)
	var workers sync.WaitGroup
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			_, recordErr := service.RecordAuthenticationResult(context.Background(), AuthenticationResultInput{
				ServerID: testServerID, EmbyUserID: "emby-user-1", AccessToken: testAccessToken,
				DeviceID: "device-1", ClientName: "Infuse",
			})
			errs <- recordErr
		}()
	}
	close(start)
	workers.Wait()
	close(errs)
	for recordErr := range errs {
		if recordErr != nil {
			t.Fatalf("concurrent RecordAuthenticationResult() error = %v", recordErr)
		}
	}

	var mappings []models.EmbyAccessToken
	if err := database.Find(&mappings).Error; err != nil {
		t.Fatalf("load token mappings: %v", err)
	}
	if len(mappings) != 1 || len(mappings[0].TokenHash) != 32 ||
		string(mappings[0].TokenHash) == testAccessToken || mappings[0].RevokedAt != nil {
		t.Fatalf("persisted mappings = %+v", mappings)
	}
	seedTokenUser(t, database, models.User{
		ID: "user_token_other", Username: "token-user-other", Role: "user",
		EmbyID: "emby-user-other", IsActive: true,
	})
	if _, err := service.RecordAuthenticationResult(context.Background(), AuthenticationResultInput{
		ServerID: testServerID, EmbyUserID: "emby-user-other", AccessToken: testAccessToken,
		DeviceID: "device-other", ClientName: "Infuse",
	}); !errors.Is(err, ErrTokenIdentityConflict) {
		t.Fatalf("RecordAuthenticationResult(identity conflict) error = %v", err)
	}
}

func TestIntegrationDeviceAndUserRevocationScopes(t *testing.T) {
	database := newTokenIntegrationDatabase(t)
	seedTokenUser(t, database, models.User{
		ID: "user_token_2", Username: "token-user-2", Role: "user",
		EmbyID: "emby-user-2", IsActive: true,
	})
	service, err := NewService(database, "fixture-token-root-key", testServerID)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	record := func(token, device string) AuthenticationMapping {
		mapping, recordErr := service.RecordAuthenticationResult(context.Background(), AuthenticationResultInput{
			ServerID: testServerID, EmbyUserID: "emby-user-2", AccessToken: token,
			DeviceID: device, ClientName: "Infuse",
		})
		if recordErr != nil {
			t.Fatalf("record token %s: %v", token, recordErr)
		}
		return mapping
	}
	tokenA1 := "fixture-token-device-a-1"
	tokenA2 := "fixture-token-device-a-2"
	tokenB := "fixture-token-device-b"
	record(tokenA1, "device-a")
	record(tokenA2, "device-a")
	mappingB := record(tokenB, "device-b")

	count, err := service.RevokeDevice(context.Background(), "user_token_2", "device-a",
		RevokeReasonManualDeviceLogout, "admin-1")
	if err != nil || count != 2 {
		t.Fatalf("RevokeDevice() count=%d error=%v", count, err)
	}
	if _, err := service.ResolvePrincipal(context.Background(), tokenA1); !errors.Is(err, ErrTokenRevoked) {
		t.Fatalf("ResolvePrincipal(device-a) error = %v", err)
	}
	if _, err := service.ResolvePrincipal(context.Background(), tokenB); err != nil {
		t.Fatalf("ResolvePrincipal(device-b) error = %v", err)
	}

	count, err = service.RevokeUserTokens(context.Background(), "user_token_2",
		RevokeReasonManualUserLogout, "admin-1")
	if err != nil || count != 1 {
		t.Fatalf("RevokeUserTokens() count=%d error=%v", count, err)
	}
	if _, err := service.ResolvePrincipal(context.Background(), tokenB); !errors.Is(err, ErrTokenRevoked) {
		t.Fatalf("ResolvePrincipal(revoked user) error = %v", err)
	}

	reactivated := record(tokenB, "device-b")
	if reactivated.ID != mappingB.ID || reactivated.RevokedAt != nil {
		t.Fatalf("reactivated mapping = %+v, original=%+v", reactivated, mappingB)
	}
	count, err = service.RevokeToken(context.Background(), reactivated.ID,
		RevokeReasonManualTokenLogout, "admin-1")
	if err != nil || count != 1 {
		t.Fatalf("RevokeToken() count=%d error=%v", count, err)
	}
}

func TestIntegrationExpiryIsDynamicAndRevokedAuditSurvivesUserDelete(t *testing.T) {
	database := newTokenIntegrationDatabase(t)
	expiredAt := time.Now().UTC().Add(-time.Hour)
	seedTokenUser(t, database, models.User{
		ID: "user_token_3", Username: "token-user-3", Role: "user",
		EmbyID: "emby-user-3", IsActive: true, ExpiresAt: &expiredAt,
	})
	service, err := NewService(database, "fixture-token-root-key", testServerID)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	mapping, err := service.RecordAuthenticationResult(context.Background(), AuthenticationResultInput{
		ServerID: testServerID, EmbyUserID: "emby-user-3", AccessToken: "fixture-expired-user-token",
		DeviceID: "device-expired", ClientName: "Infuse",
	})
	if err != nil {
		t.Fatalf("RecordAuthenticationResult() error = %v", err)
	}
	if _, err := service.ResolvePrincipal(context.Background(), "fixture-expired-user-token"); !errors.Is(err, ErrUserExpired) {
		t.Fatalf("ResolvePrincipal(expired) error = %v", err)
	}
	var persisted models.EmbyAccessToken
	if err := database.Where("id = ?", mapping.ID).First(&persisted).Error; err != nil {
		t.Fatalf("load expired mapping: %v", err)
	}
	if persisted.RevokedAt != nil {
		t.Fatalf("expiry hard-revoked mapping: %+v", persisted)
	}
	silentDB := database.Session(&gorm.Session{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err := silentDB.Delete(&models.User{}, "id = ?", "user_token_3").Error; err == nil {
		t.Fatal("active token mapping allowed user deletion without prior revocation")
	}

	count, err := service.RevokeUserTokens(context.Background(), "user_token_3",
		RevokeReasonSecurityRevoke, "system:test")
	if err != nil || count != 1 {
		t.Fatalf("RevokeUserTokens() count=%d error=%v", count, err)
	}
	if err := database.Delete(&models.User{}, "id = ?", "user_token_3").Error; err != nil {
		t.Fatalf("delete revoked user: %v", err)
	}
	if err := database.Where("id = ?", mapping.ID).First(&persisted).Error; err != nil {
		t.Fatalf("load audit mapping after user delete: %v", err)
	}
	if persisted.UserID != nil || persisted.RevokedAt == nil || persisted.RevokedReason == nil ||
		*persisted.RevokedReason != string(RevokeReasonSecurityRevoke) {
		t.Fatalf("audit mapping after user delete = %+v", persisted)
	}
}

func TestIntegrationControlPlaneRevokerScopesDeviceByUserAcrossServers(t *testing.T) {
	database := newTokenIntegrationDatabase(t)
	users := []models.User{
		{ID: "user_token_scope_1", Username: "token-scope-1", Role: "user", EmbyID: "emby-scope-1", IsActive: true},
		{ID: "user_token_scope_2", Username: "token-scope-2", Role: "user", EmbyID: "emby-scope-2", IsActive: true},
	}
	for _, user := range users {
		seedTokenUser(t, database, user)
	}
	now := time.Now().UTC()
	mappings := []models.EmbyAccessToken{
		{ID: "token_scope_1", ServerID: "server-1", TokenHash: bytes.Repeat([]byte{0x11}, 32), EmbyUserID: users[0].EmbyID, UserID: &users[0].ID, DeviceID: "shared-device", LastSeenAt: now},
		{ID: "token_scope_2", ServerID: "server-2", TokenHash: bytes.Repeat([]byte{0x12}, 32), EmbyUserID: users[0].EmbyID, UserID: &users[0].ID, DeviceID: "shared-device", LastSeenAt: now},
		{ID: "token_scope_3", ServerID: "server-1", TokenHash: bytes.Repeat([]byte{0x13}, 32), EmbyUserID: users[1].EmbyID, UserID: &users[1].ID, DeviceID: "shared-device", LastSeenAt: now},
		{ID: "token_scope_4", ServerID: "server-1", TokenHash: bytes.Repeat([]byte{0x14}, 32), EmbyUserID: users[0].EmbyID, UserID: &users[0].ID, DeviceID: "other-device", LastSeenAt: now},
	}
	if err := database.Create(&mappings).Error; err != nil {
		t.Fatalf("create mappings: %v", err)
	}
	revoker, err := NewControlPlaneRevoker(database)
	if err != nil {
		t.Fatalf("NewControlPlaneRevoker() error = %v", err)
	}
	count, err := revoker.RevokeDeviceTokens(context.Background(), users[0].ID, "shared-device", RevokeReasonManualDeviceLogout, "admin-1")
	if err != nil || count != 2 {
		t.Fatalf("RevokeDeviceTokens() count=%d error=%v", count, err)
	}
	var refreshed []models.EmbyAccessToken
	if err := database.Order("id").Find(&refreshed).Error; err != nil {
		t.Fatalf("reload mappings: %v", err)
	}
	byID := make(map[string]models.EmbyAccessToken, len(refreshed))
	for _, mapping := range refreshed {
		byID[mapping.ID] = mapping
	}
	for _, id := range []string{"token_scope_1", "token_scope_2"} {
		mapping := byID[id]
		if mapping.RevokedAt == nil || mapping.RevokedReason == nil || *mapping.RevokedReason != string(RevokeReasonManualDeviceLogout) {
			t.Fatalf("mapping %s not revoked correctly: %+v", id, mapping)
		}
	}
	for _, id := range []string{"token_scope_3", "token_scope_4"} {
		if mapping := byID[id]; mapping.RevokedAt != nil {
			t.Fatalf("mapping %s was over-revoked: %+v", id, mapping)
		}
	}
}

func newTokenIntegrationDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	baseDSN := strings.TrimSpace(os.Getenv(tokenIntegrationDatabaseEnv))
	if baseDSN == "" {
		t.Skipf("未设置 %s，跳过 Emby Token PostgreSQL 集成测试", tokenIntegrationDatabaseEnv)
	}
	adminDB := openTokenIntegrationDatabase(t, baseDSN, "")
	schemaName := fmt.Sprintf("itest_%d_embytoken", time.Now().UnixNano())
	if err := adminDB.Exec(fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS \"%s\"", schemaName)).Error; err != nil {
		t.Fatalf("create schema %s: %v", schemaName, err)
	}
	appDB := openTokenIntegrationDatabase(t, baseDSN, schemaName+",public")
	appSQLDB, err := appDB.DB()
	if err != nil {
		t.Fatalf("appDB.DB(): %v", err)
	}
	appSQLDB.SetMaxOpenConns(12)
	appSQLDB.SetMaxIdleConns(12)

	t.Setenv("EMBER_MIGRATIONS_DIR", tokenMigrationsDir(t))
	previousDB := dbpkg.DB
	dbpkg.DB = appDB
	t.Cleanup(func() {
		dbpkg.DB = previousDB
		_ = appSQLDB.Close()
		if err := adminDB.Exec(fmt.Sprintf("DROP SCHEMA IF EXISTS \"%s\" CASCADE", schemaName)).Error; err != nil {
			t.Fatalf("drop schema %s: %v", schemaName, err)
		}
		if sqlDB, err := adminDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := dbpkg.Migrate(); err != nil {
		t.Fatalf("Migrate(): %v", err)
	}
	if err := dbpkg.VerifySchema(); err != nil {
		t.Fatalf("VerifySchema(): %v", err)
	}
	assertTokenMigrationIdempotent(t, appDB)
	return appDB
}

func openTokenIntegrationDatabase(t *testing.T, dsn, searchPath string) *gorm.DB {
	t.Helper()
	var dialector gorm.Dialector = postgres.Open(dsn)
	if searchPath != "" {
		config, err := pgx.ParseConfig(dsn)
		if err != nil {
			t.Fatalf("parse integration DSN: %v", err)
		}
		config.RuntimeParams["search_path"] = searchPath
		dialector = postgres.New(postgres.Config{Conn: stdlib.OpenDB(*config)})
	}
	database, err := gorm.Open(dialector, &gorm.Config{
		DisableAutomaticPing: true,
		NowFunc:              func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	return database
}

func tokenMigrationsDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	dir := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "..", "infrastructure", "database")
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}
	return abs
}

func assertTokenMigrationIdempotent(t *testing.T, database *gorm.DB) {
	t.Helper()
	path := filepath.Join(tokenMigrationsDir(t), "20260822_03_create_emby_access_tokens.sql")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read token migration: %v", err)
	}
	if err := database.Exec(string(content)).Error; err != nil {
		t.Fatalf("reapply token migration: %v", err)
	}
}

func seedTokenUser(t *testing.T, database *gorm.DB, user models.User) {
	t.Helper()
	if strings.TrimSpace(user.Email) == "" {
		user.Email = user.Username + "@example.test"
	}
	if err := database.Create(&user).Error; err != nil {
		t.Fatalf("seed token user %s: %v", user.ID, err)
	}
}
