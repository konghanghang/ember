package app

import (
	"context"
	"github.com/konghang/ember/backend/internal/models"
	entitlementpkg "github.com/konghang/ember/backend/internal/services/entitlement"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestIntegrationWatchRetentionLifecycle verifies migration replay, actual fallback clocks and persistent invalidation.
// The harness requires a dedicated PostgreSQL URL; playback and Policy are fake.
func TestIntegrationWatchRetentionLifecycle(t *testing.T) {
	h := newIntegrationHarness(t)
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 10, 8, 15, 0, 0, 0, loc)
	a := "DEFAULT"
	rankA, rankB := 10, 20
	if err := h.database.Model(&models.PlanGroup{}).Where("key = ?", a).Updates(map[string]interface{}{"entitlement_rank": rankA, "watch_retention_enabled": true, "watch_retention_days": 30, "watch_retention_min_minutes": 60, "watch_retention_reset_at": now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := h.database.Create(&models.PlanGroup{Key: "PAID", Name: "Paid", EntitlementRank: &rankB}).Error; err != nil {
		t.Fatal(err)
	}
	user := h.seedUser(t, models.User{Username: "retention_fixture", Email: "retention@example.com", PlanGroup: &a, EmbyID: "retention-emby", IsActive: true})
	grant := func(key string, benefits []entitlementpkg.Benefit, at time.Time) {
		t.Helper()
		if err := h.database.Transaction(func(tx *gorm.DB) error {
			var locked models.User
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", user.ID).First(&locked).Error; err != nil {
				return err
			}
			return entitlementpkg.GrantLocked(tx, &locked, benefits, key, "test:fixture", at, loc)
		}); err != nil {
			t.Fatal(err)
		}
	}
	grant("watch:initial", []entitlementpkg.Benefit{{PlanGroup: a, ValidityType: "permanent"}, {PlanGroup: "PAID", ValidityType: "duration", DurationDays: 30}}, now)
	queries, syncs := 0, 0
	worker := entitlementpkg.WatchRetentionWorker{DB: h.database, Location: loc, Query: func(context.Context, string, time.Time, time.Time, *time.Location) (int64, error) {
		queries++
		return 0, nil
	}, Sync: func(string) error { syncs++; return nil }}
	if err := worker.Run(context.Background(), now.AddDate(0, 0, 20)); err != nil || queries != 0 {
		t.Fatalf("paid period queried: %v", err)
	}
	switched := now.AddDate(0, 0, 31)
	current, _, err := entitlementpkg.Reconcile(h.database, user.ID, switched)
	if err != nil || current.PlanGroup == nil || *current.PlanGroup != a {
		t.Fatalf("fallback %+v %v", current, err)
	}
	var holding models.UserEntitlement
	if err := h.database.Where("user_id = ? AND plan_group = ?", user.ID, a).First(&holding).Error; err != nil {
		t.Fatal(err)
	}
	if holding.WatchRetentionStartedAt == nil || !holding.WatchRetentionStartedAt.Equal(switched) {
		t.Fatalf("wrong start %+v", holding)
	}
	if err := worker.Run(context.Background(), switched.AddDate(0, 0, 29)); err != nil || queries != 0 {
		t.Fatalf("grace queried: %v", err)
	}
	checked, err := entitlementpkg.FirstWatchRetentionCheck(switched, 30, "0 2 * * *", loc)
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Run(context.Background(), checked); err != nil {
		t.Fatal(err)
	}
	if queries != 1 || syncs != 1 {
		t.Fatalf("queries=%d syncs=%d", queries, syncs)
	}
	h.database.Where("id = ?", user.ID).First(&current)
	if current.ResourceAccessGranted {
		t.Fatal("no-view grant retained")
	}
	current, _, err = entitlementpkg.Reconcile(h.database, user.ID, checked.Add(time.Hour))
	if err != nil || current.ResourceAccessGranted {
		t.Fatal("ordinary reconciliation revived grant")
	}
	sql, err := os.ReadFile(filepath.Join(integrationMigrationsDir(t), "20261008_01_watch_retention.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := h.database.Exec(string(sql)).Error; err != nil {
			t.Fatal(err)
		}
	}
	// Explicit repurchase restores the invalidated grant and starts a new complete period.
	recovered := checked.AddDate(0, 0, 1)
	grant("watch:repurchase", []entitlementpkg.Benefit{{PlanGroup: a, ValidityType: "permanent"}}, recovered)
	holding = models.UserEntitlement{}
	if err := h.database.Where("user_id = ? AND plan_group = ?", user.ID, a).First(&holding).Error; err != nil {
		t.Fatal(err)
	}
	if holding.WatchRetentionInvalidatedAt != nil || holding.WatchRetentionStartedAt == nil || !holding.WatchRetentionStartedAt.Equal(recovered) {
		t.Fatalf("recovery %+v", holding)
	}
}
