package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEntitlementMigrationContract checks assets and migration guards; execution requires dedicated PostgreSQL.
func TestEntitlementMigrationContract(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "infrastructure", "database", "20261004_01_plan_group_entitlements.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	for _, fragment := range []string{"CREATE TABLE IF NOT EXISTS user_entitlements", "CREATE TABLE IF NOT EXISTS entitlement_events", "entitlement_rank", "resource_access_granted", "legacy_invalidated", "ON CONFLICT", "benefits", "manual_review_reason"} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("missing %s", fragment)
		}
	}
	for _, fragment := range []string{"DELETE FROM redemptions", "TRUNCATE", "UPDATE payments SET status = 'completed'"} {
		if strings.Contains(sql, fragment) {
			t.Errorf("unsafe migration: %s", fragment)
		}
	}
}
