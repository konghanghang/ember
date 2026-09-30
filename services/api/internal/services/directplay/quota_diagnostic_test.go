package directplay

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/services/p115quota"
)

// TestTransferQuotaDiagnosticsCorrelateUserTaskAndOutcome distinguishes a
// successful Redis commit from a retained file whose quota commit failed.
func TestTransferQuotaDiagnosticsCorrelateUserTaskAndOutcome(t *testing.T) {
	for _, failCommit := range []bool{false, true} {
		t.Run(map[bool]string{false: "committed", true: "commit_failed"}[failCommit], func(t *testing.T) {
			var output bytes.Buffer
			originalWriter := log.Writer()
			log.SetOutput(&output)
			t.Cleanup(func() { log.SetOutput(originalWriter) })
			quotas := &flakyTransferStore{MemoryLeaseStore: p115quota.NewMemoryLeaseStore()}
			if failCommit {
				quotas.commitFailures = 1000
			}
			provider := newFakeProvider()
			service := newRoutedDirectPlayForTest(t, &fakeRoutedAccountRuntime{}, provider, quotas)
			service.transferCommitBudget = 10 * time.Millisecond
			service.transferRetryInterval = time.Millisecond
			_, err := service.ResolveMediaPath(context.Background(), routedMediaPathRequest("GET", "diagnostic-session"))
			if failCommit != errors.Is(err, ErrTransferQuotaCommitFailed) || (!failCommit && err != nil) {
				t.Fatalf("unexpected transfer outcome: %v", err)
			}
			code := "code=transfer_quota_committed"
			if failCommit {
				code = "code=transfer_quota_commit_failed"
			}
			var diagnostic string
			for _, line := range strings.Split(output.String(), "\n") {
				if strings.Contains(line, code) {
					diagnostic = line
					break
				}
			}
			for _, field := range []string{code, `userId="user-1"`, `taskId="task_1"`, `playbackAccountId="personal-account"`} {
				if !strings.Contains(diagnostic, field) {
					t.Fatalf("missing diagnostic field %q: %s", field, diagnostic)
				}
			}
			if failCommit {
				if !strings.Contains(diagnostic, "usageAvailable=false") || strings.Contains(diagnostic, "transferHourlyUsed=") {
					t.Fatalf("failed commit must not invent usage: %s", diagnostic)
				}
			} else {
				for _, field := range []string{"added=true", "transferPending=0", "transferHourlyUsed=1", "transferDailyUsed=1"} {
					if !strings.Contains(diagnostic, field) {
						t.Fatalf("missing committed usage %q: %s", field, diagnostic)
					}
				}
				if _, err := service.ResolveMediaPath(context.Background(), routedMediaPathRequest("GET", "diagnostic-session")); err != nil {
					t.Fatalf("cached playback reuse failed: %v", err)
				}
				if strings.Count(output.String(), "code=transfer_quota_committed ") != 1 {
					t.Fatal("playback reuse must not emit another quota commit")
				}
			}
			for _, secret := range []string{"playback-cookie", "source-cookie", provider.download.URL, directPlaySourceSHA1, "transferAttemptId="} {
				if strings.Contains(diagnostic, secret) {
					t.Fatalf("quota diagnostic exposed private data")
				}
			}
		})
	}
}
