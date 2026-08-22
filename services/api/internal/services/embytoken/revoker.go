package embytoken

import (
	"context"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
)

type controlPlaneRevocationStore interface {
	RevokeDeviceAcrossServers(context.Context, revokeInput) (int64, error)
	RevokeUser(context.Context, revokeInput) (int64, error)
}

// ControlPlaneRevoker provides local hard-state revocation without requiring
// an AccessToken, HMAC key, live Emby request or Gateway runtime ServerId.
type ControlPlaneRevoker struct {
	store controlPlaneRevocationStore
	now   func() time.Time
}

// NewControlPlaneRevoker builds the API/control-plane revocation boundary on
// the shared PostgreSQL database.
func NewControlPlaneRevoker(database *gorm.DB) (*ControlPlaneRevoker, error) {
	if database == nil {
		return nil, ErrStoreUnavailable
	}
	return newControlPlaneRevokerWithStore(&gormMappingStore{db: database}), nil
}

// newControlPlaneRevokerWithStore isolates persistence for state-flow tests.
func newControlPlaneRevokerWithStore(store controlPlaneRevocationStore) *ControlPlaneRevoker {
	return &ControlPlaneRevoker{store: store, now: time.Now}
}

// RevokeDeviceTokens revokes one user's device across all historical ServerId
// values while preserving other users that happen to reuse the same DeviceId.
func (revoker *ControlPlaneRevoker) RevokeDeviceTokens(
	ctx context.Context,
	userID, deviceID string,
	reason RevokeReason,
	actor string,
) (int64, error) {
	input, err := revoker.normalizedInput(ctx, userID, reason, actor)
	if err != nil {
		return 0, err
	}
	input.DeviceID = strings.TrimSpace(deviceID)
	if !validBoundedValue(input.DeviceID, maxDeviceIDLength, false) {
		return 0, ErrInvalidInput
	}
	count, err := revoker.store.RevokeDeviceAcrossServers(ctx, input)
	if err != nil {
		return 0, err
	}
	log.Printf("[EmbyTokenRevoker] scope=device userId=%s deviceId=%s reason=%s count=%d",
		input.UserID, input.DeviceID, input.Reason, count)
	return count, nil
}

// RevokeUserTokens revokes every active mapping for one Ember user across all
// ServerId and device values.
func (revoker *ControlPlaneRevoker) RevokeUserTokens(
	ctx context.Context,
	userID string,
	reason RevokeReason,
	actor string,
) (int64, error) {
	input, err := revoker.normalizedInput(ctx, userID, reason, actor)
	if err != nil {
		return 0, err
	}
	count, err := revoker.store.RevokeUser(ctx, input)
	if err != nil {
		return 0, err
	}
	log.Printf("[EmbyTokenRevoker] scope=user userId=%s reason=%s count=%d", input.UserID, input.Reason, count)
	return count, nil
}

// normalizedInput validates common audit values without ever accepting raw
// Token material.
func (revoker *ControlPlaneRevoker) normalizedInput(
	ctx context.Context,
	userID string,
	reason RevokeReason,
	actor string,
) (revokeInput, error) {
	if revoker == nil || revoker.store == nil || ctx == nil {
		return revokeInput{}, ErrStoreUnavailable
	}
	userID = strings.TrimSpace(userID)
	actor = strings.TrimSpace(actor)
	if !validBoundedValue(userID, maxInternalIDLength, false) ||
		!validBoundedValue(actor, maxRevokedByLength, false) {
		return revokeInput{}, ErrInvalidInput
	}
	if !validRevokeReason(reason) {
		return revokeInput{}, ErrRevokeReasonInvalid
	}
	return revokeInput{UserID: userID, Reason: reason, RevokedBy: actor, At: revoker.now().UTC()}, nil
}
