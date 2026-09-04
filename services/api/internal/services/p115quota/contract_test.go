package p115quota

import (
	"errors"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/security/tokenhash"
)

func TestLeaseDurationsMatchPlanContract(t *testing.T) {
	if ReservationTTL != 30*time.Second || ActiveTTL != 2*time.Minute || PausedTTL != 15*time.Minute {
		t.Fatalf("lease TTLs = %v/%v/%v", ReservationTTL, ActiveTTL, PausedTTL)
	}
	if LeaseIndexTTL != 16*time.Minute || TransferPendingTTL != 5*time.Minute || TransferPendingKeyTTL != 6*time.Minute {
		t.Fatalf("physical TTLs = %v/%v/%v", LeaseIndexTTL, TransferPendingTTL, TransferPendingKeyTTL)
	}
}

func TestKeyDeriverUsesCanonicalProviderIdentity(t *testing.T) {
	deriver, err := NewKeyDeriver("fixture-root-key")
	if err != nil {
		t.Fatalf("NewKeyDeriver() error = %v", err)
	}
	first, err := deriver.PlaybackAccountKey("00100")
	if err != nil {
		t.Fatalf("PlaybackAccountKey() error = %v", err)
	}
	second, err := deriver.PlaybackAccountKey("100")
	if err != nil {
		t.Fatalf("PlaybackAccountKey(canonical) error = %v", err)
	}
	if first != second || len(first) != 64 {
		t.Fatalf("account keys = %q/%q, want same 64-char digest", first, second)
	}
	if _, err := deriver.PlaybackAccountKey("0"); !errors.Is(err, ErrProviderUserIDInvalid) {
		t.Fatalf("PlaybackAccountKey(0) error = %v", err)
	}
	if _, err := deriver.PlaybackAccountKey("raw-user-id"); !errors.Is(err, ErrProviderUserIDInvalid) {
		t.Fatalf("PlaybackAccountKey(raw) error = %v", err)
	}
}

func TestKeyDeriverSeparatesAccountAndSessionPurposes(t *testing.T) {
	deriver, err := NewKeyDeriver("fixture-root-key")
	if err != nil {
		t.Fatalf("NewKeyDeriver() error = %v", err)
	}
	accountKey, err := deriver.PlaybackAccountKey("100")
	if err != nil {
		t.Fatalf("PlaybackAccountKey() error = %v", err)
	}
	sessionKey, err := deriver.SessionFingerprint(SessionIdentity{
		ServerID:      "server-1",
		UserID:        "user-1",
		MappingID:     "mapping-1",
		DeviceID:      "device-1",
		PlaySessionID: "session-1",
	})
	if err != nil {
		t.Fatalf("SessionFingerprint() error = %v", err)
	}
	if sessionKey == accountKey || len(sessionKey) != 64 {
		t.Fatalf("session key = %q, account key = %q", sessionKey, accountKey)
	}
	_, err = deriver.SessionFingerprint(SessionIdentity{ServerID: "server-1"})
	if !errors.Is(err, ErrSessionIdentityInvalid) {
		t.Fatalf("SessionFingerprint(incomplete) error = %v", err)
	}
	if _, err := NewKeyDeriver(""); !errors.Is(err, tokenhash.ErrKeyMissing) {
		t.Fatalf("NewKeyDeriver(empty) error = %v", err)
	}
}
