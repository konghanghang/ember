package p115account

import (
	"context"

	"gorm.io/gorm"
)

// ControlPlaneRevoker exposes only credential erasure needed by user deletion.
// It cannot decrypt credentials or touch Redis leases.
type ControlPlaneRevoker struct {
	store *gormAccountStore
}

// NewControlPlaneRevoker builds the narrow user-deletion dependency.
func NewControlPlaneRevoker(database *gorm.DB) (*ControlPlaneRevoker, error) {
	if database == nil {
		return nil, ErrStoreUnavailable
	}
	return &ControlPlaneRevoker{store: &gormAccountStore{db: database}}, nil
}

// RevokePersonalAccount writes the irreversible tombstone for one user.
func (r *ControlPlaneRevoker) RevokePersonalAccount(ctx context.Context, userID string) error {
	return r.store.RevokePersonal(ctx, userID)
}
