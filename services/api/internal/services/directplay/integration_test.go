package directplay

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	dbpkg "github.com/konghang/ember/backend/internal/db"
	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/services/p115account"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const directPlayIntegrationDatabaseEnv = "EMBER_INTEGRATION_DATABASE_URL"

func TestIntegrationConcurrentResolveUsesOneRapidUpload(t *testing.T) {
	database := newDirectPlayIntegrationDatabase(t)
	accounts := seedDirectPlayAccounts(t, database)
	provider := newConcurrentTransferProvider()
	service, err := NewService(database, accounts, provider)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	start := make(chan struct{})
	results := make(chan RedirectCandidate, 2)
	errorsCh := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			result, resolveErr := service.ResolveMediaPath(context.Background(), fixtureMediaPathResolveRequest())
			results <- result
			errorsCh <- resolveErr
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	close(errorsCh)

	for resolveErr := range errorsCh {
		if resolveErr != nil {
			t.Fatalf("concurrent Resolve() error = %v", resolveErr)
		}
	}
	createdCount := 0
	preexistingCount := 0
	for result := range results {
		if result.URL == "" {
			t.Fatalf("concurrent Resolve() missing URL: %+v", result)
		}
		if result.TaskID != "" {
			createdCount++
		}
		if result.Preexisting {
			preexistingCount++
		}
	}
	if createdCount != 1 || preexistingCount != 1 {
		t.Fatalf("concurrent results created=%d preexisting=%d", createdCount, preexistingCount)
	}
	if provider.initCount.Load() != 1 {
		t.Fatalf("InitRapidUpload() calls = %d, want 1", provider.initCount.Load())
	}

	var tasks []models.PlaybackTransferTask
	if err := database.Order("created_at ASC").Find(&tasks).Error; err != nil {
		t.Fatalf("load playback transfer tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Status != models.PlaybackTransferTaskStatusSucceeded ||
		tasks[0].TargetFileID == nil || *tasks[0].TargetFileID != provider.targetFile.ID ||
		tasks[0].TargetPickCode == nil || *tasks[0].TargetPickCode != provider.targetFile.PickCode ||
		tasks[0].CompletedAt == nil || tasks[0].LastAccessedAt == nil ||
		tasks[0].LastAccessedAt.Before(*tasks[0].CompletedAt) {
		t.Fatalf("persisted task = %+v", tasks)
	}
}

func TestIntegrationChallengePersistsSecondAttempt(t *testing.T) {
	database := newDirectPlayIntegrationDatabase(t)
	accounts := seedDirectPlayAccounts(t, database)
	provider := newFakeProvider()
	provider.searchResults = [][]p115integration.File{{}, {}}
	provider.uploadResults = []p115integration.RapidUploadResult{
		{
			Status: p115integration.RapidUploadRangeChallenge,
			Challenge: &p115integration.RapidUploadChallenge{
				Range:   p115integration.ByteRange{Start: 10, End: 19},
				SignKey: "fixture-sign-key",
			},
		},
		{Status: p115integration.RapidUploadReused},
	}
	service, err := NewService(database, accounts, provider)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.ResolveMediaPath(context.Background(), fixtureMediaPathResolveRequest())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.TaskID == "" || result.Preexisting {
		t.Fatalf("Resolve() result = %+v", result)
	}
	var task models.PlaybackTransferTask
	if err := database.Where("id = ?", result.TaskID).First(&task).Error; err != nil {
		t.Fatalf("load challenge task: %v", err)
	}
	if task.Status != models.PlaybackTransferTaskStatusSucceeded || task.AttemptCount != 2 ||
		task.CompletedAt == nil || task.LastAccessedAt == nil {
		t.Fatalf("challenge task = %+v", task)
	}
}

func TestIntegrationOrdinaryUploadPersistsFailedTask(t *testing.T) {
	database := newDirectPlayIntegrationDatabase(t)
	accounts := seedDirectPlayAccounts(t, database)
	provider := newFakeProvider()
	provider.searchResults = [][]p115integration.File{{}, {}}
	provider.uploadResults = []p115integration.RapidUploadResult{{Status: p115integration.RapidUploadOrdinaryUploadRequired}}
	service, err := NewService(database, accounts, provider)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.ResolveMediaPath(context.Background(), fixtureMediaPathResolveRequest())
	if !errors.Is(err, ErrRapidUploadUnavailable) {
		t.Fatalf("Resolve() error = %v, want ErrRapidUploadUnavailable", err)
	}
	var tasks []models.PlaybackTransferTask
	if err := database.Find(&tasks).Error; err != nil {
		t.Fatalf("load failed transfer task: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Status != models.PlaybackTransferTaskStatusFailed ||
		tasks[0].LastErrorCode == nil || *tasks[0].LastErrorCode != "ordinary_upload_required" ||
		tasks[0].CompletedAt == nil || tasks[0].TargetFileID != nil || tasks[0].TargetPickCode != nil {
		t.Fatalf("failed task = %+v", tasks)
	}
}

func TestIntegrationProviderFailureCoolsAccountAndSkipsNextProviderCall(t *testing.T) {
	database := newDirectPlayIntegrationDatabase(t)
	accounts := seedDirectPlayAccounts(t, database)
	provider := newFakeProvider()
	provider.resolveErr = p115integration.ErrProviderUnavailable
	service, err := NewService(database, accounts, provider)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.ResolveMediaPath(context.Background(), fixtureMediaPathResolveRequest())
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("first ResolveMediaPath() error = %v, want ErrProviderUnavailable", err)
	}
	var source models.P115Account
	if err := database.Where("role = ?", models.P115AccountRoleSource).First(&source).Error; err != nil {
		t.Fatalf("load source account: %v", err)
	}
	if source.Status != models.P115AccountStatusCoolingDown || !source.Enabled || source.CooldownUntil == nil ||
		!source.CooldownUntil.After(time.Now().UTC()) || source.LastErrorCode == nil || *source.LastErrorCode != "provider_unavailable" {
		t.Fatalf("source runtime health = %+v", source)
	}
	if countCalls(provider.calls, "resolve_source") != 1 {
		t.Fatalf("provider calls after first failure = %v", provider.calls)
	}

	_, err = service.ResolveMediaPath(context.Background(), fixtureMediaPathResolveRequest())
	if !errors.Is(err, ErrAccountUnavailable) {
		t.Fatalf("second ResolveMediaPath() error = %v, want ErrAccountUnavailable", err)
	}
	if countCalls(provider.calls, "resolve_source") != 1 {
		t.Fatalf("cooling request reached provider: %v", provider.calls)
	}
}

func TestIntegrationCredentialRejectedExpiresAndDisablesAccount(t *testing.T) {
	database := newDirectPlayIntegrationDatabase(t)
	accounts := seedDirectPlayAccounts(t, database)
	provider := newFakeProvider()
	provider.resolveErr = p115integration.ErrCredentialRejected
	service, err := NewService(database, accounts, provider)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.ResolveMediaPath(context.Background(), fixtureMediaPathResolveRequest())
	if !errors.Is(err, ErrAccountUnavailable) {
		t.Fatalf("ResolveMediaPath() error = %v, want ErrAccountUnavailable", err)
	}
	var source models.P115Account
	if err := database.Where("role = ?", models.P115AccountRoleSource).First(&source).Error; err != nil {
		t.Fatalf("load source account: %v", err)
	}
	if source.Status != models.P115AccountStatusExpired || source.Enabled || source.CooldownUntil != nil ||
		source.LastErrorCode == nil || *source.LastErrorCode != "credential_rejected" {
		t.Fatalf("source rejected state = %+v", source)
	}
}

func TestIntegrationExpiredCooldownAllowsOnlyOneProbeLease(t *testing.T) {
	database := newDirectPlayIntegrationDatabase(t)
	accounts := seedDirectPlayAccounts(t, database)
	past := time.Now().UTC().Add(-time.Minute)
	if err := database.Model(&models.P115Account{}).
		Where("role = ?", models.P115AccountRoleSource).
		Updates(map[string]interface{}{
			"status":         models.P115AccountStatusCoolingDown,
			"cooldown_until": past,
			"updated_at":     past,
		}).Error; err != nil {
		t.Fatalf("seed expired cooldown: %v", err)
	}

	start := make(chan struct{})
	errorsCh := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			_, loadErr := accounts.LoadActiveCredentialByRole(context.Background(), models.P115AccountRoleSource)
			errorsCh <- loadErr
		}()
	}
	close(start)
	workers.Wait()
	close(errorsCh)

	successes := 0
	cooling := 0
	for loadErr := range errorsCh {
		switch {
		case loadErr == nil:
			successes++
		case errors.Is(loadErr, p115account.ErrAccountCoolingDown):
			cooling++
		default:
			t.Fatalf("LoadActiveCredentialByRole() error = %v", loadErr)
		}
	}
	if successes != 1 || cooling != 1 {
		t.Fatalf("probe lease results success=%d cooling=%d", successes, cooling)
	}
}

func TestIntegrationSuccessfulProbeClearsExpiredCooldown(t *testing.T) {
	database := newDirectPlayIntegrationDatabase(t)
	accounts := seedDirectPlayAccounts(t, database)
	past := time.Now().UTC().Add(-time.Minute)
	oldCode := "provider_unavailable"
	oldMessage := "115 服务暂不可用"
	if err := database.Model(&models.P115Account{}).
		Where("role = ?", models.P115AccountRoleSource).
		Updates(map[string]interface{}{
			"status":             models.P115AccountStatusCoolingDown,
			"cooldown_until":     past,
			"last_error_code":    oldCode,
			"last_error_message": oldMessage,
			"updated_at":         past,
		}).Error; err != nil {
		t.Fatalf("seed expired cooldown: %v", err)
	}
	provider := newFakeProvider()
	provider.searchResults = [][]p115integration.File{{provider.targetFile}}
	service, err := NewService(database, accounts, provider)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if _, err := service.ResolveMediaPath(context.Background(), fixtureMediaPathResolveRequest()); err != nil {
		t.Fatalf("ResolveMediaPath() error = %v", err)
	}
	var source models.P115Account
	if err := database.Where("role = ?", models.P115AccountRoleSource).First(&source).Error; err != nil {
		t.Fatalf("load source account: %v", err)
	}
	if source.Status != models.P115AccountStatusActive || source.CooldownUntil != nil || source.LastSucceededAt == nil ||
		source.LastErrorCode != nil || source.LastErrorMessage != nil {
		t.Fatalf("source recovered state = %+v", source)
	}
}

func TestIntegrationStaleRuntimeFailureCannotOverrideReplacedCookie(t *testing.T) {
	database := newDirectPlayIntegrationDatabase(t)
	accounts := seedDirectPlayAccounts(t, database)
	active, err := accounts.LoadActiveCredentialByRole(context.Background(), models.P115AccountRoleSource)
	if err != nil {
		t.Fatalf("LoadActiveCredentialByRole() error = %v", err)
	}
	if _, err := accounts.ReplaceCookie(context.Background(), active.Credential.AccountID, p115account.ReplaceCookieInput{
		Cookie: "UID=100_F1_1700000000; CID=replaced",
	}); err != nil {
		t.Fatalf("ReplaceCookie() error = %v", err)
	}

	err = accounts.ReportRuntimeHealth(context.Background(), active, p115account.RuntimeHealthProviderUnavailable)
	if !errors.Is(err, p115account.ErrRuntimeStateChanged) {
		t.Fatalf("ReportRuntimeHealth() error = %v, want ErrRuntimeStateChanged", err)
	}
	var source models.P115Account
	if err := database.Where("id = ?", active.Credential.AccountID).First(&source).Error; err != nil {
		t.Fatalf("load replaced source account: %v", err)
	}
	if source.Status != models.P115AccountStatusPending || source.Enabled || source.CooldownUntil != nil || source.LastErrorCode != nil {
		t.Fatalf("stale runtime failure changed replacement state: %+v", source)
	}
}

func countCalls(calls []string, want string) int {
	count := 0
	for _, call := range calls {
		if call == want {
			count++
		}
	}
	return count
}

type concurrentTransferProvider struct {
	*fakeProvider
	initialSearchBarrier chan struct{}
	searchCount          int
	uploaded             bool
	initCount            atomic.Int32
}

func newConcurrentTransferProvider() *concurrentTransferProvider {
	return &concurrentTransferProvider{
		fakeProvider:         newFakeProvider(),
		initialSearchBarrier: make(chan struct{}),
	}
}

func (provider *concurrentTransferProvider) SearchBySHA1(_ context.Context, _ p115integration.Credential, _ p115integration.FileQuery) ([]p115integration.File, error) {
	provider.mu.Lock()
	provider.calls = append(provider.calls, "search_target")
	provider.searchCount++
	call := provider.searchCount
	if call == 2 {
		close(provider.initialSearchBarrier)
	}
	uploaded := provider.uploaded
	provider.mu.Unlock()
	if call <= 2 {
		<-provider.initialSearchBarrier
		return []p115integration.File{}, nil
	}
	if uploaded {
		return []p115integration.File{provider.targetFile}, nil
	}
	return []p115integration.File{}, nil
}

func (provider *concurrentTransferProvider) InitRapidUpload(_ context.Context, _ p115integration.Credential, request p115integration.RapidUploadRequest) (p115integration.RapidUploadResult, error) {
	provider.mu.Lock()
	provider.calls = append(provider.calls, "init_upload")
	provider.uploadRequests = append(provider.uploadRequests, request)
	provider.uploaded = true
	provider.mu.Unlock()
	provider.initCount.Add(1)
	return p115integration.RapidUploadResult{Status: p115integration.RapidUploadReused}, nil
}

func newDirectPlayIntegrationDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	baseDSN := strings.TrimSpace(os.Getenv(directPlayIntegrationDatabaseEnv))
	if baseDSN == "" {
		t.Skipf("未设置 %s，跳过 direct play PostgreSQL 集成测试", directPlayIntegrationDatabaseEnv)
	}

	adminDB := openDirectPlayIntegrationDatabase(t, baseDSN, "")
	schemaName := fmt.Sprintf("itest_%d_directplay", time.Now().UnixNano())
	if err := adminDB.Exec(fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS "%s"`, schemaName)).Error; err != nil {
		t.Fatalf("create schema %s: %v", schemaName, err)
	}
	appDB := openDirectPlayIntegrationDatabase(t, baseDSN, schemaName+",public")
	appSQLDB, err := appDB.DB()
	if err != nil {
		t.Fatalf("appDB.DB(): %v", err)
	}
	appSQLDB.SetMaxOpenConns(8)
	appSQLDB.SetMaxIdleConns(8)

	t.Setenv("EMBER_MIGRATIONS_DIR", directPlayMigrationsDir(t))
	previousDB := dbpkg.DB
	dbpkg.DB = appDB
	t.Cleanup(func() {
		dbpkg.DB = previousDB
		_ = appSQLDB.Close()
		if err := adminDB.Exec(fmt.Sprintf(`DROP SCHEMA IF EXISTS "%s" CASCADE`, schemaName)).Error; err != nil {
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
	assertDirectPlayMigrationsIdempotent(t, appDB)
	return appDB
}

func assertDirectPlayMigrationsIdempotent(t *testing.T, database *gorm.DB) {
	t.Helper()
	for _, filename := range []string{
		"20260822_01_create_playback_transfer_tasks.sql",
		"20260822_02_add_p115_source_location.sql",
		"20260822_03_create_emby_access_tokens.sql",
	} {
		path := filepath.Join(directPlayMigrationsDir(t), filename)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", filename, err)
		}
		if err := database.Exec(string(content)).Error; err != nil {
			t.Fatalf("reapply migration %s: %v", filename, err)
		}
	}
}

func openDirectPlayIntegrationDatabase(t *testing.T, dsn, searchPath string) *gorm.DB {
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

func directPlayMigrationsDir(t *testing.T) string {
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

func seedDirectPlayAccounts(t *testing.T, database *gorm.DB) *p115account.Service {
	t.Helper()
	service, err := p115account.NewService(database, strings.Repeat("k", 32), integrationCredentialValidator{})
	if err != nil {
		t.Fatalf("p115account.NewService(): %v", err)
	}
	source, err := service.Create(context.Background(), p115account.CreateAccountInput{
		Role: models.P115AccountRoleSource, Alias: "source", Cookie: "source-cookie",
		AppType: "ios", UserAgent: "fixture-agent",
		EmbyPathPrefix: "/mnt/cloudNAS/115lifetime", SourceRootID: "0",
	})
	if err != nil {
		t.Fatalf("create source account: %v", err)
	}
	playback, err := service.Create(context.Background(), p115account.CreateAccountInput{
		Role: models.P115AccountRolePlayback, Alias: "playback", Cookie: "playback-cookie",
		AppType: "ios", UserAgent: "fixture-agent", TargetParentID: "200000002",
	})
	if err != nil {
		t.Fatalf("create playback account: %v", err)
	}
	now := time.Now().UTC()
	activate := func(id, providerUserID string) {
		if err := database.Model(&models.P115Account{}).Where("id = ?", id).Updates(map[string]interface{}{
			"provider_user_id":  providerUserID,
			"status":            models.P115AccountStatusActive,
			"enabled":           true,
			"last_validated_at": now,
			"updated_at":        now,
		}).Error; err != nil {
			t.Fatalf("activate p115 account %s: %v", id, err)
		}
	}
	activate(source.ID, "provider-source")
	activate(playback.ID, "provider-playback")
	return service
}

type integrationCredentialValidator struct{}

func (integrationCredentialValidator) ValidateCredential(context.Context, p115integration.Credential) (p115integration.AccountIdentity, error) {
	return p115integration.AccountIdentity{}, nil
}
