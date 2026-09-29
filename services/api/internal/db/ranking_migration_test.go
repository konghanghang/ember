package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRankingBatchMigrationContract 保护迁移资产和启动指纹；SQL 执行另由 PostgreSQL 集成覆盖。
func TestRankingBatchMigrationContract(t *testing.T) {
	const migration = "20260929_01_playback_ranking_batches"
	content, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "infrastructure", "database", migration+".sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS playback_ranking_batches",
		"DROP INDEX IF EXISTS uq_playback_rankings_period",
		"uq_playback_ranking_batches_period",
		"uq_playback_rankings_batch_rank",
		"fk_playback_rankings_batch",
		"ON CONFLICT",
	} {
		if !strings.Contains(sql, required) {
			t.Errorf("migration missing %q", required)
		}
	}
	for _, forbidden := range []string{"DELETE FROM playback_rankings", "TRUNCATE", "CREATE INDEX CONCURRENTLY"} {
		if strings.Contains(sql, forbidden) {
			t.Errorf("migration must preserve historical rows and run in a transaction: %s", forbidden)
		}
	}
	wanted := map[string]bool{"uq_playback_ranking_batches_period": false, "uq_playback_rankings_batch_rank": false}
	for _, index := range schemaFingerprintIndexes {
		if index.index == "uq_playback_rankings_period" {
			t.Error("schema fingerprint still requires the invalid detail-period unique index")
		}
		if _, ok := wanted[index.index]; ok && index.migration == migration {
			wanted[index.index] = true
		}
	}
	for index, found := range wanted {
		if !found {
			t.Errorf("missing migration fingerprint %s", index)
		}
	}
}
