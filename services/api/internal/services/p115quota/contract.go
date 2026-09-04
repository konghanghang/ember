// Package p115quota defines the Redis-backed playback lease and transfer-quota contract.
package p115quota

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/security/tokenhash"
)

const (
	ReservationTTL        = 30 * time.Second
	ActiveTTL             = 2 * time.Minute
	PausedTTL             = 15 * time.Minute
	LeaseIndexTTL         = 16 * time.Minute
	TransferPendingTTL    = 5 * time.Minute
	TransferPendingKeyTTL = 6 * time.Minute

	playbackAccountKeyPurpose = "p115-playback-account-key"
	sessionFingerprintPurpose = "p115-playback-session-fingerprint"
)

var (
	ErrProviderUserIDInvalid  = errors.New("115 Provider 用户标识无效")
	ErrSessionIdentityInvalid = errors.New("115 播放会话身份无效")
)

// LeaseState is the current Redis lease state for one playback session.
type LeaseState string

const (
	LeaseStateReservation LeaseState = "reservation"
	LeaseStateActive      LeaseState = "active"
	LeaseStatePaused      LeaseState = "paused"
)

// SessionIdentity contains the stable non-secret identities used to correlate
// repeated video requests with successful Emby playback events.
type SessionIdentity struct {
	ServerID      string
	UserID        string
	MappingID     string
	DeviceID      string
	PlaySessionID string
}

// KeyDeriver creates purpose-separated opaque identifiers for Redis keys.
type KeyDeriver struct {
	accountHasher *tokenhash.Hasher
	sessionHasher *tokenhash.Hasher
}

// NewKeyDeriver derives Redis key material from the deployment encryption root.
func NewKeyDeriver(rootKey string) (*KeyDeriver, error) {
	accountHasher, err := tokenhash.New(rootKey, playbackAccountKeyPurpose)
	if err != nil {
		return nil, err
	}
	sessionHasher, err := tokenhash.New(rootKey, sessionFingerprintPurpose)
	if err != nil {
		return nil, err
	}
	return &KeyDeriver{accountHasher: accountHasher, sessionHasher: sessionHasher}, nil
}

// PlaybackAccountKey hashes a canonical positive decimal Provider UID. Database
// account IDs and Ember owner IDs intentionally do not participate.
func (d *KeyDeriver) PlaybackAccountKey(providerUserID string) (string, error) {
	canonical, err := canonicalProviderUserID(providerUserID)
	if err != nil {
		return "", err
	}
	digest, err := d.accountHasher.Sum(canonical)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(digest), nil
}

// SessionFingerprint hashes a length-prefixed identity tuple so field
// boundaries remain unambiguous without exposing raw session identifiers.
func (d *KeyDeriver) SessionFingerprint(identity SessionIdentity) (string, error) {
	fields := []string{identity.ServerID, identity.UserID, identity.MappingID, identity.DeviceID, identity.PlaySessionID}
	for _, field := range fields {
		if field == "" || strings.TrimSpace(field) != field || strings.ContainsAny(field, "\r\n") {
			return "", ErrSessionIdentityInvalid
		}
	}
	encoded := ""
	for _, field := range fields {
		encoded += fmt.Sprintf("%d:%s", len(field), field)
	}
	digest, err := d.sessionHasher.Sum(encoded)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(digest), nil
}

func canonicalProviderUserID(raw string) (string, error) {
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || value == 0 {
		return "", ErrProviderUserIDInvalid
	}
	return strconv.FormatUint(value, 10), nil
}
