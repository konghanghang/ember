package entitlement

import (
	"github.com/konghang/ember/backend/internal/models"
	"testing"
	"time"
)

// TestWatchRetentionInvalidationCannotRevive protects fallback, unrelated grants and explicit repurchase.
func TestWatchRetentionInvalidationCannotRevive(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 10, 8, 15, 0, 0, 0, loc)
	ranks := map[string]int{"A": 10, "B": 20}
	owned := []Holding{{PlanGroup: "A", ValidityType: Permanent, WatchRetentionInvalidatedAt: &now}}
	got, err := Resolve(owned, ranks, now)
	if err != nil || got != nil {
		t.Fatalf("invalidated grant revived: %+v %v", got, err)
	}
	next, err := Grant(owned, []Benefit{{PlanGroup: "B", ValidityType: Duration, DurationDays: 1}}, ranks, now, loc)
	if err != nil || next[0].WatchRetentionInvalidatedAt == nil {
		t.Fatalf("unrelated purchase reset A: %+v %v", next, err)
	}
	next, err = Grant(owned, []Benefit{{PlanGroup: "A", ValidityType: Permanent}}, ranks, now, loc)
	if err != nil || next[0].WatchRetentionInvalidatedAt != nil {
		t.Fatalf("repurchase did not recover A: %+v %v", next, err)
	}
}

// TestWatchRetentionSchedule covers configurable cron and exact calendar boundaries.
func TestWatchRetentionSchedule(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	start := time.Date(2026, 10, 8, 15, 0, 0, 0, loc)
	first, err := FirstWatchRetentionCheck(start, 30, "0 2 * * *", loc)
	want := time.Date(2026, 11, 8, 2, 0, 0, 0, loc)
	if err != nil || !first.Equal(want) {
		t.Fatalf("first=%s err=%v", first, err)
	}
	first, err = FirstWatchRetentionCheck(start, 14, "0 16 * * *", loc)
	if err != nil || !first.Equal(time.Date(2026, 10, 22, 16, 0, 0, 0, loc)) {
		t.Fatalf("custom=%s err=%v", first, err)
	}
	exact := time.Date(2026, 10, 8, 2, 0, 0, 0, loc)
	first, err = FirstWatchRetentionCheck(exact, 30, "0 2 * * *", loc)
	if err != nil || !first.Equal(exact.AddDate(0, 0, 30)) {
		t.Fatalf("exact boundary=%s %v", first, err)
	}
	if _, err = FirstWatchRetentionCheck(start, 30, "bad", loc); err == nil {
		t.Fatal("accepted invalid cron")
	}
}

// TestRetentionPolicyResetAndSnapshot covers paused/restarted grants and stale query rejection.
func TestRetentionPolicyResetAndSnapshot(t *testing.T) {
	now := time.Now()
	old := now.AddDate(0, 0, -10)
	holding := models.UserEntitlement{WatchRetentionStartedAt: &old}
	group := models.PlanGroup{Key: "A", WatchRetentionResetAt: &now, WatchRetentionDays: 30, WatchRetentionMinMinutes: 60}
	if !RetentionStart(holding, group).Equal(now) {
		t.Fatal("policy did not restart period")
	}
	holding.WatchRetentionInvalidatedAt = &old
	if RetentionStart(holding, group) != nil {
		t.Fatal("policy revived invalidated grant")
	}
	a := &retentionSnapshot{User: models.User{EmbyID: "emby"}, Group: group, Start: now}
	b := *a
	if !sameRetentionSnapshot(a, &b) {
		t.Fatal("identical snapshot rejected")
	}
	b.Start = old
	if sameRetentionSnapshot(a, &b) {
		t.Fatal("accepted stale grace")
	}
	b = *a
	b.User.EmbyID = "new"
	if sameRetentionSnapshot(a, &b) {
		t.Fatal("accepted changed identity")
	}
	b = *a
	b.Group.WatchRetentionMinMinutes = 120
	if sameRetentionSnapshot(a, &b) {
		t.Fatal("accepted changed policy")
	}
}

// TestRetentionScheduleUsesGlobalTimezone rejects per-expression overrides and preserves DST calendar semantics.
func TestRetentionScheduleUsesGlobalTimezone(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	start := time.Date(2026, 3, 7, 12, 0, 0, 0, loc)
	next, err := FirstWatchRetentionCheck(start, 1, "0 12 * * *", loc)
	if err != nil || next.Sub(start) != 23*time.Hour {
		t.Fatalf("calendar boundary: %s %v", next, err)
	}
	if _, err := FirstWatchRetentionCheck(start, 30, "CRON_TZ=UTC 0 2 * * *", loc); err == nil {
		t.Fatal("accepted timezone override")
	}
	old := registeredRetentionSchedule.Load()
	t.Cleanup(func() { registeredRetentionSchedule.Store(old) })
	RegisterRetentionSchedule("0 3 * * *", loc, true)
	if got := RegisteredRetentionSchedule(); got.Expression != "0 3 * * *" || got.Location != loc || !got.Enabled {
		t.Fatalf("schedule %+v", got)
	}
}
