package p115account

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/logging"
	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/services/p115quota"
)

// TestPersonalUsageDiagnosticsShowReadSnapshotOnlyAtDebug covers real zeroes,
// unavailable counters and the configured business-day window without leaking keys.
func TestPersonalUsageDiagnosticsShowReadSnapshotOnlyAtDebug(t *testing.T) {
	originalWriter := log.Writer()
	originalLevel := "info"
	if logging.DebugEnabled() {
		originalLevel = "debug"
	}
	var output bytes.Buffer
	log.SetOutput(&output)
	t.Cleanup(func() {
		log.SetOutput(originalWriter)
		_ = logging.ApplyLevel(originalLevel)
	})
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, location)
	service := newServiceWithDependencies(&fakeAccountStore{personalPolicy: PersonalPlanPolicy{
		PlanGroupKey: "VIP", PlaybackMode: models.P115PlaybackModeSystem, TransferHourlyLimit: 5, TransferDailyLimit: 10,
	}}, fakeCredentialCipher{})
	quotas := p115quota.NewMemoryLeaseStore()
	service.leases = quotas
	service.businessTimezone = location
	service.now = func() time.Time { return now }
	if err := logging.ApplyLevel("info"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetPersonalUsage(context.Background(), "user-1"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "code=p115_user_usage_read") {
		t.Fatal("successful usage reads must stay hidden at info")
	}
	if err := logging.ApplyLevel("debug"); err != nil {
		t.Fatal(err)
	}
	summary, err := service.GetPersonalUsage(context.Background(), "user-1")
	if err != nil || !summary.UsageAvailable {
		t.Fatalf("summary=%+v error=%v", summary, err)
	}
	diagnostic := output.String()
	for _, field := range []string{"code=p115_user_usage_read", `userId="user-1"`, "playbackMode=system", "usageAvailable=true", "userOccupiedStreams=0", "transferHourlyUsed=0", "transferDailyUsed=0", `businessTimezone="Asia/Shanghai"`, "dayStartUnixMs=1790697600000", "dayEndUnixMs=1790784000000"} {
		if !strings.Contains(diagnostic, field) {
			t.Fatalf("missing field %q: %s", field, diagnostic)
		}
	}
	output.Reset()
	accountKey := strings.Repeat("a", 64)
	fingerprint := strings.Repeat("b", 64)
	if _, err := quotas.Reserve(context.Background(), p115quota.ReserveRequest{
		PlaybackAccountKey: accountKey, UserID: "user-1", SessionFingerprint: fingerprint, MaxConcurrentStreams: 2,
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := quotas.Advance(context.Background(), fingerprint, p115quota.LeaseStateActive, now); err != nil {
		t.Fatal(err)
	}
	dayStart, dayEnd := p115quota.DayWindow(now, location)
	if _, err := quotas.CommitTransfer(context.Background(), p115quota.TransferCommitRequest{
		UserID: "user-1", AttemptID: "diagnostic-attempt", DayStart: dayStart, DayEnd: dayEnd,
	}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetPersonalUsage(context.Background(), "user-1"); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"userActiveStreams=1", "userOccupiedStreams=1", "transferHourlyUsed=1", "transferDailyUsed=1"} {
		if !strings.Contains(output.String(), field) {
			t.Fatalf("nonzero read omitted %q: %s", field, output.String())
		}
	}
	for _, secret := range []string{accountKey, fingerprint, "diagnostic-attempt", "transferAttemptId="} {
		if strings.Contains(output.String(), secret) {
			t.Fatal("usage read exposed a Redis key or attempt identity")
		}
	}
	output.Reset()
	service.leases = p115quota.UnavailableLeaseStore{}
	summary, err = service.GetPersonalUsage(context.Background(), "user-1")
	if err != nil || summary.UsageAvailable {
		t.Fatalf("unavailable summary=%+v error=%v", summary, err)
	}
	if strings.Contains(output.String(), "code=p115_user_usage_read ") {
		t.Fatal("failed read must not log a successful zero snapshot")
	}
}
