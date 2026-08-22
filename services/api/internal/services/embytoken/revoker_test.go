package embytoken

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestControlPlaneRevokerScopesDeviceAcrossServersByUser(t *testing.T) {
	store := &fakeControlPlaneRevocationStore{deviceCount: 2}
	revoker := newControlPlaneRevokerWithStore(store)
	now := time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC)
	revoker.now = func() time.Time { return now }

	count, err := revoker.RevokeDeviceTokens(context.Background(), " user-1 ", " device-1 ", RevokeReasonManualDeviceLogout, " admin-1 ")
	if err != nil || count != 2 {
		t.Fatalf("RevokeDeviceTokens() count=%d error=%v", count, err)
	}
	if store.deviceInput.UserID != "user-1" || store.deviceInput.DeviceID != "device-1" ||
		store.deviceInput.ServerID != "" || store.deviceInput.Reason != RevokeReasonManualDeviceLogout ||
		store.deviceInput.RevokedBy != "admin-1" || !store.deviceInput.At.Equal(now) {
		t.Fatalf("device revoke input = %+v", store.deviceInput)
	}
}

func TestControlPlaneRevokerRevokesUserAcrossServers(t *testing.T) {
	store := &fakeControlPlaneRevocationStore{userCount: 3}
	revoker := newControlPlaneRevokerWithStore(store)
	now := time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC)
	revoker.now = func() time.Time { return now }

	count, err := revoker.RevokeUserTokens(context.Background(), " user-1 ", RevokeReasonUserDeleted, " admin-1 ")
	if err != nil || count != 3 {
		t.Fatalf("RevokeUserTokens() count=%d error=%v", count, err)
	}
	if store.userInput.UserID != "user-1" || store.userInput.Reason != RevokeReasonUserDeleted ||
		store.userInput.RevokedBy != "admin-1" || !store.userInput.At.Equal(now) {
		t.Fatalf("user revoke input = %+v", store.userInput)
	}
}

func TestControlPlaneRevokerRejectsInvalidInputBeforeStore(t *testing.T) {
	store := &fakeControlPlaneRevocationStore{}
	revoker := newControlPlaneRevokerWithStore(store)
	tests := []struct {
		name   string
		invoke func() error
		want   error
	}{
		{name: "missing user", invoke: func() error {
			_, err := revoker.RevokeUserTokens(context.Background(), "", RevokeReasonUserDisabled, "admin-1")
			return err
		}, want: ErrInvalidInput},
		{name: "missing device", invoke: func() error {
			_, err := revoker.RevokeDeviceTokens(context.Background(), "user-1", "", RevokeReasonManualDeviceLogout, "admin-1")
			return err
		}, want: ErrInvalidInput},
		{name: "invalid reason", invoke: func() error {
			_, err := revoker.RevokeUserTokens(context.Background(), "user-1", RevokeReason("other"), "admin-1")
			return err
		}, want: ErrRevokeReasonInvalid},
		{name: "missing actor", invoke: func() error {
			_, err := revoker.RevokeUserTokens(context.Background(), "user-1", RevokeReasonUserDisabled, "")
			return err
		}, want: ErrInvalidInput},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.invoke(); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
	if store.deviceCalls != 0 || store.userCalls != 0 {
		t.Fatalf("store calls device=%d user=%d", store.deviceCalls, store.userCalls)
	}
}

type fakeControlPlaneRevocationStore struct {
	deviceInput revokeInput
	userInput   revokeInput
	deviceCount int64
	userCount   int64
	deviceCalls int
	userCalls   int
	err         error
}

func (store *fakeControlPlaneRevocationStore) RevokeDeviceAcrossServers(_ context.Context, input revokeInput) (int64, error) {
	store.deviceCalls++
	store.deviceInput = input
	return store.deviceCount, store.err
}

func (store *fakeControlPlaneRevocationStore) RevokeUser(_ context.Context, input revokeInput) (int64, error) {
	store.userCalls++
	store.userInput = input
	return store.userCount, store.err
}
