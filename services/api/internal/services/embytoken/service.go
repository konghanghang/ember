package embytoken

import (
	"context"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/security/tokenhash"
	"gorm.io/gorm"
)

const (
	tokenHashPurpose      = "emby-access-token"
	lastSeenWriteInterval = 5 * time.Minute
	maxServerIDLength     = 64
	maxEmbyUserIDLength   = 50
	maxDeviceIDLength     = 256
	maxClientNameLength   = 128
	maxRevokedByLength    = 64
	maxInternalIDLength   = 25
)

// RevokeReason is a stable audit value for local Playback Gateway revocation.
type RevokeReason string

const (
	RevokeReasonManualTokenLogout  RevokeReason = "manual_token_logout"
	RevokeReasonManualDeviceLogout RevokeReason = "manual_device_logout"
	RevokeReasonManualUserLogout   RevokeReason = "manual_user_logout"
	RevokeReasonUserDisabled       RevokeReason = "user_disabled"
	RevokeReasonEmbyDisabled       RevokeReason = "emby_disabled"
	RevokeReasonEmbyAccessDisabled RevokeReason = "emby_access_disabled"
	RevokeReasonEmbyUnbound        RevokeReason = "emby_unbound"
	RevokeReasonSecurityRevoke     RevokeReason = "security_revoke"
)

// AuthenticationResultInput is the fixed subset extracted from a successful
// Emby 4.9.3.0 AuthenticationResult plus non-authoritative device metadata.
type AuthenticationResultInput struct {
	ServerID    string
	EmbyUserID  string
	AccessToken string
	DeviceID    string
	ClientName  string
}

// AuthenticationMapping is safe to return internally and intentionally omits
// both the original AccessToken and its digest.
type AuthenticationMapping struct {
	ID            string
	ServerID      string
	EmbyUserID    string
	UserID        string
	DeviceID      string
	ClientName    string
	LastSeenAt    time.Time
	RevokedAt     *time.Time
	RevokedReason *string
	RevokedBy     *string
}

// Principal is the resolved Ember identity for one mapped Emby AccessToken.
type Principal struct {
	MappingID  string
	ServerID   string
	DeviceID   string
	ClientName string
	User       models.User
}

type tokenHasher interface {
	Sum(token string) ([]byte, error)
}

type upsertMappingInput struct {
	ServerID   string
	TokenHash  []byte
	EmbyUserID string
	UserID     string
	DeviceID   string
	ClientName string
	At         time.Time
}

type revokeInput struct {
	MappingID string
	ServerID  string
	UserID    string
	DeviceID  string
	Reason    RevokeReason
	RevokedBy string
	At        time.Time
}

type mappingStore interface {
	FindUserByEmbyID(ctx context.Context, embyUserID string) (*models.User, error)
	FindUserByID(ctx context.Context, userID string) (*models.User, error)
	UpsertMapping(ctx context.Context, input upsertMappingInput) (*models.EmbyAccessToken, error)
	FindMapping(ctx context.Context, serverID string, tokenHash []byte) (*models.EmbyAccessToken, error)
	TouchLastSeen(ctx context.Context, mappingID string, at, cutoff time.Time) error
	RevokeToken(ctx context.Context, input revokeInput) (int64, error)
	RevokeDevice(ctx context.Context, input revokeInput) (int64, error)
	RevokeUser(ctx context.Context, input revokeInput) (int64, error)
}

// Service owns one-way token identity mapping and local revocation semantics.
type Service struct {
	store            mappingStore
	hasher           tokenHasher
	expectedServerID string
	now              func() time.Time
}

// NewService builds the production mapping service without reading environment
// variables or performing an Emby request.
func NewService(database *gorm.DB, encryptionKey, expectedServerID string) (*Service, error) {
	if database == nil {
		return nil, ErrStoreUnavailable
	}
	expectedServerID = strings.TrimSpace(expectedServerID)
	if !validBoundedValue(expectedServerID, maxServerIDLength, false) {
		return nil, ErrInvalidInput
	}
	hasher, err := tokenhash.New(encryptionKey, tokenHashPurpose)
	if err != nil {
		return nil, err
	}
	return newServiceWithDependencies(&gormMappingStore{db: database}, hasher, expectedServerID), nil
}

// newServiceWithDependencies isolates hashing and persistence in unit tests.
func newServiceWithDependencies(store mappingStore, hasher tokenHasher, expectedServerID string) *Service {
	return &Service{store: store, hasher: hasher, expectedServerID: expectedServerID, now: time.Now}
}

// RecordAuthenticationResult binds one successful Emby authentication result
// to the uniquely linked Ember user and reactivates the same mapping only on a
// new successful authentication.
func (service *Service) RecordAuthenticationResult(ctx context.Context, input AuthenticationResultInput) (AuthenticationMapping, error) {
	normalized, err := normalizeAuthenticationInput(input)
	if err != nil {
		return AuthenticationMapping{}, err
	}
	if normalized.ServerID != service.expectedServerID {
		return AuthenticationMapping{}, ErrServerMismatch
	}
	digest, err := service.hasher.Sum(normalized.AccessToken)
	if err != nil {
		return AuthenticationMapping{}, ErrInvalidInput
	}
	user, err := service.store.FindUserByEmbyID(ctx, normalized.EmbyUserID)
	if err != nil {
		return AuthenticationMapping{}, err
	}
	if user.EmbyID != normalized.EmbyUserID {
		return AuthenticationMapping{}, ErrIdentityMismatch
	}
	if hardDisabled(user) {
		return AuthenticationMapping{}, ErrUserUnavailable
	}
	mapping, err := service.store.UpsertMapping(ctx, upsertMappingInput{
		ServerID: normalized.ServerID, TokenHash: digest, EmbyUserID: normalized.EmbyUserID,
		UserID: user.ID, DeviceID: normalized.DeviceID, ClientName: normalized.ClientName,
		At: service.now().UTC(),
	})
	if err != nil {
		return AuthenticationMapping{}, err
	}
	log.Printf("[EmbyToken] 认证映射记录 mappingId=%s userId=%s serverId=%s", mapping.ID, user.ID, mapping.ServerID)
	return mappingView(mapping), nil
}

// ResolvePrincipal maps an exact raw Token to a current Ember user and applies
// live hard-state and expiry checks before returning the principal.
func (service *Service) ResolvePrincipal(ctx context.Context, accessToken string) (Principal, error) {
	digest, err := service.hasher.Sum(accessToken)
	if err != nil {
		return Principal{}, ErrInvalidInput
	}
	mapping, err := service.store.FindMapping(ctx, service.expectedServerID, digest)
	if err != nil {
		return Principal{}, err
	}
	if mapping.RevokedAt != nil || mapping.UserID == nil || strings.TrimSpace(*mapping.UserID) == "" {
		return Principal{}, ErrTokenRevoked
	}
	user, err := service.store.FindUserByID(ctx, *mapping.UserID)
	if err != nil {
		return Principal{}, err
	}
	if user.EmbyID != mapping.EmbyUserID {
		return Principal{}, ErrIdentityMismatch
	}
	if hardDisabled(user) {
		return Principal{}, ErrUserUnavailable
	}
	now := service.now().UTC()
	if user.ExpiresAt != nil && user.ExpiresAt.Before(now) {
		return Principal{}, ErrUserExpired
	}
	cutoff := now.Add(-lastSeenWriteInterval)
	if mapping.LastSeenAt.Before(cutoff) {
		if err := service.store.TouchLastSeen(ctx, mapping.ID, now, cutoff); err != nil {
			return Principal{}, err
		}
	}
	return Principal{
		MappingID: mapping.ID, ServerID: mapping.ServerID, DeviceID: mapping.DeviceID,
		ClientName: mapping.ClientName, User: *user,
	}, nil
}

// RevokeToken locally revokes one mapped login.
func (service *Service) RevokeToken(ctx context.Context, mappingID string, reason RevokeReason, revokedBy string) (int64, error) {
	input, err := service.revokeInput(reason, revokedBy)
	if err != nil {
		return 0, err
	}
	input.MappingID = strings.TrimSpace(mappingID)
	if !validBoundedValue(input.MappingID, maxInternalIDLength, false) {
		return 0, ErrInvalidInput
	}
	return service.store.RevokeToken(ctx, input)
}

// RevokeDevice locally revokes every active mapping for one user/device pair on
// the configured Emby Server.
func (service *Service) RevokeDevice(ctx context.Context, userID, deviceID string, reason RevokeReason, revokedBy string) (int64, error) {
	input, err := service.revokeInput(reason, revokedBy)
	if err != nil {
		return 0, err
	}
	input.UserID = strings.TrimSpace(userID)
	input.DeviceID = strings.TrimSpace(deviceID)
	if !validBoundedValue(input.UserID, maxInternalIDLength, false) ||
		!validBoundedValue(input.DeviceID, maxDeviceIDLength, false) {
		return 0, ErrInvalidInput
	}
	return service.store.RevokeDevice(ctx, input)
}

// RevokeUserTokens locally revokes every active mapping for one Ember user.
func (service *Service) RevokeUserTokens(ctx context.Context, userID string, reason RevokeReason, revokedBy string) (int64, error) {
	input, err := service.revokeInput(reason, revokedBy)
	if err != nil {
		return 0, err
	}
	input.UserID = strings.TrimSpace(userID)
	if !validBoundedValue(input.UserID, maxInternalIDLength, false) {
		return 0, ErrInvalidInput
	}
	return service.store.RevokeUser(ctx, input)
}

// revokeInput normalizes common audit fields for every revocation scope.
func (service *Service) revokeInput(reason RevokeReason, revokedBy string) (revokeInput, error) {
	if !validRevokeReason(reason) {
		return revokeInput{}, ErrRevokeReasonInvalid
	}
	revokedBy = strings.TrimSpace(revokedBy)
	if !validBoundedValue(revokedBy, maxRevokedByLength, false) {
		return revokeInput{}, ErrInvalidInput
	}
	return revokeInput{
		ServerID: service.expectedServerID, Reason: reason, RevokedBy: revokedBy,
		At: service.now().UTC(),
	}, nil
}

// normalizeAuthenticationInput preserves the exact Token while bounding every
// database-backed identity and rejecting header-injection characters.
func normalizeAuthenticationInput(input AuthenticationResultInput) (AuthenticationResultInput, error) {
	input.ServerID = strings.TrimSpace(input.ServerID)
	input.EmbyUserID = strings.TrimSpace(input.EmbyUserID)
	input.DeviceID = strings.TrimSpace(input.DeviceID)
	input.ClientName = strings.TrimSpace(input.ClientName)
	if !validBoundedValue(input.ServerID, maxServerIDLength, false) ||
		!validBoundedValue(input.EmbyUserID, maxEmbyUserIDLength, false) ||
		!validBoundedValue(input.DeviceID, maxDeviceIDLength, true) ||
		!validBoundedValue(input.ClientName, maxClientNameLength, true) ||
		input.AccessToken == "" || strings.TrimSpace(input.AccessToken) != input.AccessToken ||
		strings.ContainsAny(input.AccessToken, "\r\n") {
		return AuthenticationResultInput{}, ErrInvalidInput
	}
	return input, nil
}

// validBoundedValue enforces UTF-8, byte-length and line-boundary constraints.
func validBoundedValue(value string, maxLength int, allowEmpty bool) bool {
	if value == "" {
		return allowEmpty
	}
	return utf8.ValidString(value) && len(value) <= maxLength && !strings.ContainsAny(value, "\r\n")
}

// hardDisabled identifies user states that must never create or resolve an
// active local Token mapping.
func hardDisabled(user *models.User) bool {
	return user == nil || !user.IsActive || user.EmbyDisabled || user.EmbyAccessDisabled ||
		strings.TrimSpace(user.EmbyID) == ""
}

// validRevokeReason limits persisted audit values to the fixed contract.
func validRevokeReason(reason RevokeReason) bool {
	switch reason {
	case RevokeReasonManualTokenLogout, RevokeReasonManualDeviceLogout, RevokeReasonManualUserLogout,
		RevokeReasonUserDisabled, RevokeReasonEmbyDisabled, RevokeReasonEmbyAccessDisabled,
		RevokeReasonEmbyUnbound, RevokeReasonSecurityRevoke:
		return true
	default:
		return false
	}
}

// mappingView removes the digest and normalizes the nullable user relation for
// internal callers.
func mappingView(mapping *models.EmbyAccessToken) AuthenticationMapping {
	if mapping == nil {
		return AuthenticationMapping{}
	}
	userID := ""
	if mapping.UserID != nil {
		userID = *mapping.UserID
	}
	return AuthenticationMapping{
		ID: mapping.ID, ServerID: mapping.ServerID, EmbyUserID: mapping.EmbyUserID,
		UserID: userID, DeviceID: mapping.DeviceID, ClientName: mapping.ClientName,
		LastSeenAt: mapping.LastSeenAt, RevokedAt: mapping.RevokedAt,
		RevokedReason: mapping.RevokedReason, RevokedBy: mapping.RevokedBy,
	}
}
