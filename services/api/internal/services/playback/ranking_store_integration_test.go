package playback

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	configpkg "github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// rankingIntegrationStore 仅连接显式指定的隔离测试库，每个用例创建独立 schema。
// applyAll=false 用于从真实旧 baseline 验证升级；不启动服务、不装配任何外部客户端。
func rankingIntegrationStore(t *testing.T, applyAll bool) (*gorm.DB, string) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("EMBER_INTEGRATION_DATABASE_URL"))
	if dsn == "" {
		t.Skip("未设置 EMBER_INTEGRATION_DATABASE_URL，跳过真实 PostgreSQL 排行批次验证")
	}
	schema := "ranking_" + strings.ToLower(generateRankingBatchID())
	open := func(searchPath string) *gorm.DB {
		configuration, err := pgx.ParseConfig(dsn)
		if err != nil {
			t.Fatal("专用集成数据库配置无效")
		}
		configuration.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
		if searchPath != "" {
			configuration.RuntimeParams["search_path"] = searchPath
		}
		connection := stdlib.OpenDB(*configuration)
		connection.SetMaxOpenConns(4)
		connection.SetMaxIdleConns(4)
		database, err := gorm.Open(postgres.New(postgres.Config{Conn: connection}), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent), NowFunc: func() time.Time { return time.Now().UTC() },
		})
		if err != nil {
			_ = connection.Close()
			t.Fatal("无法连接专用集成数据库")
		}
		t.Cleanup(func() { _ = connection.Close() })
		return database
	}
	admin := open("")
	if err := admin.Exec(fmt.Sprintf(`CREATE SCHEMA "%s"`, schema)).Error; err != nil {
		t.Fatal("创建集成测试 schema 失败")
	}
	t.Cleanup(func() {
		if err := admin.Exec(fmt.Sprintf(`DROP SCHEMA "%s" CASCADE`, schema)).Error; err != nil {
			t.Error("清理集成测试 schema 失败")
		}
	})
	// 不回退到 public，避免幂等迁移中的 DROP INDEX 命中测试 schema 之外的同名对象。
	database := open(schema)
	previous := db.DB
	db.DB = database
	configpkg.InvalidateCachedSetting("CRON_TIMEZONE")
	t.Cleanup(func() {
		db.DB = previous
		configpkg.InvalidateCachedSetting("CRON_TIMEZONE")
	})
	directory, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "..", "infrastructure", "database"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("EMBER_MIGRATIONS_DIR", directory)
	if applyAll {
		if err := db.Migrate(); err != nil {
			t.Fatal(err)
		}
		if err := db.VerifySchema(); err != nil {
			t.Fatal(err)
		}
	} else {
		executeRankingMigration(t, database, directory, "00000000_baseline_20260605.sql")
	}
	return database, directory
}

// executeRankingMigration 与生产迁移器一样在事务里执行完整 SQL 文件。
func executeRankingMigration(t *testing.T, database *gorm.DB, directory, name string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(directory, name))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Transaction(func(tx *gorm.DB) error { return tx.Exec(string(content)).Error }); err != nil {
		t.Fatalf("migration %s failed: %v", name, err)
	}
}

// TestIntegrationRankingBatchConcurrentWrites 验证真实唯一约束在竞争下仅保留完整的一个批次。
func TestIntegrationRankingBatchConcurrentWrites(t *testing.T) {
	database, _ := rankingIntegrationStore(t, true)
	var wait sync.WaitGroup
	results := make(chan bool, 4)
	errors := make(chan error, 4)
	start := make(chan struct{})
	for i := 0; i < 4; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			fixture := rankingStoreFixture(20)
			fixture.BatchID = generateRankingBatchID()
			<-start
			created, err := persistRankingBatch(fixture)
			results <- created
			errors <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	close(errors)
	winners := 0
	for created := range results {
		if created {
			winners++
		}
	}
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var batches []models.PlaybackRankingBatch
	var items []models.PlaybackRanking
	if err := database.Find(&batches).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Find(&items).Error; err != nil {
		t.Fatal(err)
	}
	if winners != 1 || len(batches) != 1 || len(items) != 20 || batches[0].TotalDuration == nil || *batches[0].TotalDuration != 3600 {
		t.Fatalf("winners=%d batches=%d items=%d", winners, len(batches), len(items))
	}
	for _, item := range items {
		if item.BatchID != batches[0].ID {
			t.Fatal("concurrent candidates mixed detail batches")
		}
	}
}

// TestIntegrationRankingBatchRollbackAndRetry 验证明细约束失败不会留下阻挡重试的空批次。
func TestIntegrationRankingBatchRollbackAndRetry(t *testing.T) {
	database, _ := rankingIntegrationStore(t, true)
	fixture := rankingStoreFixture(20)
	fixture.Movies[1].Rank = 1
	if created, err := persistRankingBatch(fixture); err == nil || created {
		t.Fatal("duplicate rank should fail atomically")
	}
	var count int64
	if err := database.Model(&models.PlaybackRankingBatch{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("failed write left a batch: count=%d err=%v", count, err)
	}
	fixture.Movies[1].Rank = 2
	if created, err := persistRankingBatch(fixture); err != nil || !created {
		t.Fatalf("retry created=%v err=%v", created, err)
	}
	if err := database.Model(&models.PlaybackRanking{}).Count(&count).Error; err != nil || count != 20 {
		t.Fatalf("retry items=%d err=%v", count, err)
	}
}

// TestIntegrationRankingBatchEmptyLatestAndFuture 验证空榜覆盖上一期，未结束周期立即可见且不提前展示未来批次。
func TestIntegrationRankingBatchEmptyLatestAndFuture(t *testing.T) {
	database, _ := rankingIntegrationStore(t, true)
	if err := database.Save(&models.Setting{Key: "CRON_TIMEZONE", Value: "Asia/Singapore"}).Error; err != nil {
		t.Fatal(err)
	}
	configpkg.InvalidateCachedSetting("CRON_TIMEZONE")
	loc := configpkg.LoadConfiguredTimezone()
	now := time.Now().In(loc).Truncate(time.Microsecond)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	previous := rankingStoreFixture(1)
	previous.BatchID = "previous_batch"
	previous.Start, previous.End, previous.ComputedAt = start.AddDate(0, 0, -1), start, now.AddDate(0, 0, -1)
	if _, err := persistRankingBatch(previous); err != nil {
		t.Fatal(err)
	}
	fixture := rankingStoreFixture(0)
	fixture.Start, fixture.End, fixture.ComputedAt = start, start.AddDate(0, 0, 1), now
	if created, err := persistRankingBatch(fixture); err != nil || !created {
		t.Fatalf("empty batch created=%v err=%v", created, err)
	}
	if created, err := persistRankingBatch(fixture); err != nil || created {
		t.Fatalf("duplicate empty batch created=%v err=%v", created, err)
	}
	future := *fixture
	future.BatchID = "future_generated"
	future.Start = start.AddDate(0, 0, -1)
	future.End = start.AddDate(0, 0, 2)
	future.ComputedAt = now.Add(24 * time.Hour)
	if _, err := persistRankingBatch(&future); err != nil {
		t.Fatal(err)
	}
	result, err := (&PlaybackRankingService{}).GetLatestRanking(models.RankingDaily)
	if err != nil || result == nil || result.BatchID != fixture.BatchID || len(result.Movies) != 0 || result.Movies == nil || result.Episodes == nil {
		t.Fatalf("unexpected latest empty batch: result=%+v err=%v", result, err)
	}
	if !result.PeriodEnd.Equal(fixture.End) || !result.SnapshotAt.Equal(now) {
		t.Fatal("batch metadata was lost")
	}
}

// TestIntegrationRankingBatchMigrationPreservesHistory 覆盖旧批次/无批次数据升级和重复执行，不虚构历史总量。
func TestIntegrationRankingBatchMigrationPreservesHistory(t *testing.T) {
	database, directory := rankingIntegrationStore(t, false)
	fixture := rankingStoreFixture(1)
	row := fixture.Movies[0]
	row.Period, row.PeriodStart, row.PeriodEnd, row.SnapshotAt = fixture.Period, fixture.Start, fixture.End, fixture.ComputedAt
	row.BatchID = "old_batch"
	if err := database.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	row.ID, row.BatchID = "legacy_row", ""
	row.PeriodStart, row.PeriodEnd, row.SnapshotAt = row.PeriodStart.AddDate(0, 0, -1), row.PeriodEnd.AddDate(0, 0, -1), row.SnapshotAt.AddDate(0, 0, -1)
	if err := database.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	const migration = "20260929_01_playback_ranking_batches.sql"
	for i := 0; i < 2; i++ {
		executeRankingMigration(t, database, directory, migration)
	}
	var batches []models.PlaybackRankingBatch
	if err := database.Order("id").Find(&batches).Error; err != nil {
		t.Fatal(err)
	}
	if len(batches) != 2 || batches[0].ID != "legacy_legacy_row" || batches[1].ID != "old_batch" {
		t.Fatalf("unexpected migrated batches: %+v", batches)
	}
	for _, batch := range batches {
		if batch.TotalDuration != nil {
			t.Fatal("migration fabricated historical total duration")
		}
		result, err := (&PlaybackRankingService{}).GetHistoryRanking(batch.Period, batch.PeriodStart, batch.PeriodEnd)
		if err != nil || result == nil || len(result.Movies) != 1 || result.BatchID != batch.ID {
			t.Fatalf("history lost after migration: %+v err=%v", result, err)
		}
	}
	fixture.Start, fixture.End = fixture.Start.AddDate(0, 0, 1), fixture.End.AddDate(0, 0, 1)
	fixture.Movies[0].ID = "new_row"
	if _, err := persistRankingBatch(fixture); err != nil {
		t.Fatal(err)
	}
	executeRankingMigration(t, database, directory, migration)
	var reloaded models.PlaybackRankingBatch
	if err := database.First(&reloaded, "id = ?", fixture.BatchID).Error; err != nil || reloaded.TotalDuration == nil || *reloaded.TotalDuration != 3600 {
		t.Fatalf("rerun damaged new batch total: %+v err=%v", reloaded, err)
	}
}
