package playbackgateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/config"
)

type compatibilityLimitsPolicy struct {
	transferTriggerPolicyFunc
	readLimits func(context.Context) (config.CompatibilityTransferLimits, error)
}

// P115CompatibilityTransferLimits injects configuration failures and proof
// replacement at the same boundary used by production locked admission.
func (p compatibilityLimitsPolicy) P115CompatibilityTransferLimits(ctx context.Context) (config.CompatibilityTransferLimits, error) {
	return p.readLimits(ctx)
}

// TestCompatibilityAuthorizationLimitsFailClosed verifies the total 500ms
// configuration budget, fresh proof checks and explicit-intent independence.
func TestCompatibilityAuthorizationLimitsFailClosed(t *testing.T) {
	for _, kind := range []string{"valid", "load_failure", "replaced", "explicit"} {
		t.Run(kind, func(t *testing.T) {
			gateway, proof, _ := newTransferTriggerFixture(t, nil, kind == "explicit")
			var modeDeadline time.Time
			reads := 0
			gateway.transferTriggerPolicy = compatibilityLimitsPolicy{
				transferTriggerPolicyFunc: func(ctx context.Context) (string, error) {
					modeDeadline, _ = ctx.Deadline()
					return "compatible", nil
				},
				readLimits: func(ctx context.Context) (config.CompatibilityTransferLimits, error) {
					reads++
					deadline, _ := ctx.Deadline()
					if !deadline.Equal(modeDeadline) {
						t.Error("mode and limits got separate timeout budgets")
					}
					if kind == "load_failure" || kind == "explicit" {
						return config.CompatibilityTransferLimits{}, errors.New("fixture-policy-secret")
					}
					if kind == "replaced" {
						gateway.proofs.Record([]PlaybackProof{proof})
					}
					return config.CompatibilityTransferLimits{UserHourly: 1, UserDaily: 3, AccountHourly: 10, AccountDaily: 30}, nil
				},
			}
			authorization, trigger := gateway.authorizeVideoTransfer(context.Background(), "GET", proof, fixturePrincipal())
			want := kind == "valid" || kind == "explicit"
			if authorization.Allowed != want {
				t.Fatalf("authorization=%+v trigger=%s", authorization, trigger)
			}
			if kind == "valid" && (authorization.CompatibilityLimits == nil || authorization.CompatibilityLimits.UserHourly != 1) {
				t.Fatal("weak GET lost its sublimit")
			}
			if kind == "explicit" && (reads != 0 || authorization.CompatibilityLimits != nil) {
				t.Fatal("explicit intent unnecessarily depends on weak-GET configuration")
			}
		})
	}
}
