package directplay

import (
	"context"
	"errors"
	"net/http"
	"testing"

	p115 "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/services/p115quota"
)

// TestCompatibilityQuotaLifecycle covers the real service's total/sublimit
// reservation, success bookkeeping, and failure refund before later retries.
func TestCompatibilityQuotaLifecycle(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{true: "provider_failure", false: "success"}[fail], func(t *testing.T) {
			provider := newFakeProvider()
			leases := p115quota.NewMemoryLeaseStore()
			service := newRoutedDirectPlayForTest(t, &fakeRoutedAccountRuntime{}, provider, leases)
			request := routedMediaPathRequest(http.MethodGet, "compat-session")
			request.AuthorizeTransfer = func() TransferAuthorization {
				return TransferAuthorization{Allowed: true, CompatibilityLimits: &CompatibilityTransferLimits{UserHourly: 1, UserDaily: 3, AccountHourly: 10, AccountDaily: 30}}
			}
			if fail {
				provider.uploadResults = []p115.RapidUploadResult{{Status: p115.RapidUploadProviderRejected}}
			}
			_, err := service.ResolveMediaPath(context.Background(), request)
			if (err != nil) != fail {
				t.Fatalf("resolve error=%v", err)
			}
			key, _ := service.keyDeriver.PlaybackAccountKey("100")
			start, end := p115quota.DayWindow(service.now(), service.businessTimezone)
			for _, id := range []string{"user-1", "compat-user-user-1", "compat-account-" + key} {
				usage, err := leases.TransferUsage(context.Background(), p115quota.TransferUsageRequest{UserID: id, DayStart: start, DayEnd: end}, service.now())
				want := 1
				if fail {
					want = 0
				}
				if err != nil || usage.Pending != 0 || usage.HourlyUsed != want || usage.DailyUsed != want {
					t.Fatalf("scope=%s usage=%+v error=%v", id, usage, err)
				}
			}
		})
	}
}

// TestCompatibilityQuotaStopsBeforeProviderWrite proves an exhausted weak-GET
// budget cannot create a task, read preID or upload, while total quota remains.
func TestCompatibilityQuotaStopsBeforeProviderWrite(t *testing.T) {
	provider := newFakeProvider()
	leases := p115quota.NewMemoryLeaseStore()
	service := newRoutedDirectPlayForTest(t, &fakeRoutedAccountRuntime{}, provider, leases)
	start, end := p115quota.DayWindow(service.now(), service.businessTimezone)
	_, err := leases.ReserveTransfer(context.Background(), p115quota.TransferReserveRequest{UserID: "compat-user-user-1", AttemptID: "prior-probe", HourlyLimit: 1, DailyLimit: 3, DayStart: start, DayEnd: end}, service.now())
	if err != nil {
		t.Fatal(err)
	}
	request := routedMediaPathRequest("GET", "blocked-compat")
	request.AuthorizeTransfer = func() TransferAuthorization {
		return TransferAuthorization{Allowed: true, CompatibilityLimits: &CompatibilityTransferLimits{UserHourly: 1, UserDaily: 3, AccountHourly: 10, AccountDaily: 30}}
	}
	_, err = service.ResolveMediaPath(context.Background(), request)
	if !errors.Is(err, ErrCompatibilityQuotaExceeded) {
		t.Fatalf("error=%v", err)
	}
	if service.store.(*fakeTaskStore).beginCount != 0 || len(provider.rangeRequests) != 0 || len(provider.uploadRequests) != 0 {
		t.Fatal("denied compatibility created transfer work")
	}
	usage, _ := leases.TransferUsage(context.Background(), p115quota.TransferUsageRequest{UserID: "user-1", DayStart: start, DayEnd: end}, service.now())
	if usage.Pending != 0 || usage.DailyUsed != 0 {
		t.Fatalf("denied compatibility consumed total: %+v", usage)
	}
}
