package embytoken

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/security/tokenhash"
)

const (
	testServerID    = "emby-server-1"
	testAccessToken = "fixture-emby-access-token"
)

func TestRecordAuthenticationResultHashesAndBindsUser(t *testing.T) {
	store := newFakeStore()
	store.usersByEmbyID["emby-user-1"] = &models.User{
		ID: "user-1", EmbyID: "emby-user-1", IsActive: true,
	}
	hasher, _ := tokenhash.New("fixture-root-key", tokenHashPurpose)
	service := newServiceWithDependencies(store, hasher, testServerID)
	now := time.Date(2026, 8, 22, 8, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	result, err := service.RecordAuthenticationResult(context.Background(), AuthenticationResultInput{
		ServerID: testServerID, EmbyUserID: "emby-user-1", AccessToken: testAccessToken,
		DeviceID: "device-1", ClientName: "Infuse",
	})
	if err != nil {
		t.Fatalf("RecordAuthenticationResult() error = %v", err)
	}
	if result.UserID != "user-1" || result.EmbyUserID != "emby-user-1" || result.DeviceID != "device-1" {
		t.Fatalf("RecordAuthenticationResult() = %+v", result)
	}
	if store.upsertInput.UserID != "user-1" || store.upsertInput.ServerID != testServerID ||
		len(store.upsertInput.TokenHash) != 32 || string(store.upsertInput.TokenHash) == testAccessToken {
		t.Fatalf("upsert input = %+v", store.upsertInput)
	}
	if _, ok := reflect.TypeOf(AuthenticationMapping{}).FieldByName("AccessToken"); ok {
		t.Fatal("AuthenticationMapping must not expose AccessToken")
	}
}

func TestRecordAuthenticationResultRejectsServerMismatchAndHardDisabledUser(t *testing.T) {
	hasher, _ := tokenhash.New("fixture-root-key", tokenHashPurpose)
	tests := []struct {
		name    string
		input   AuthenticationResultInput
		user    models.User
		wantErr error
	}{
		{
			name:    "server mismatch",
			input:   AuthenticationResultInput{ServerID: "other", EmbyUserID: "emby-user-1", AccessToken: testAccessToken},
			user:    models.User{ID: "user-1", EmbyID: "emby-user-1", IsActive: true},
			wantErr: ErrServerMismatch,
		},
		{
			name:    "inactive user",
			input:   AuthenticationResultInput{ServerID: testServerID, EmbyUserID: "emby-user-1", AccessToken: testAccessToken},
			user:    models.User{ID: "user-1", EmbyID: "emby-user-1", IsActive: false},
			wantErr: ErrUserUnavailable,
		},
		{
			name:    "emby access disabled",
			input:   AuthenticationResultInput{ServerID: testServerID, EmbyUserID: "emby-user-1", AccessToken: testAccessToken},
			user:    models.User{ID: "user-1", EmbyID: "emby-user-1", IsActive: true, EmbyAccessDisabled: true},
			wantErr: ErrUserUnavailable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newFakeStore()
			user := test.user
			store.usersByEmbyID["emby-user-1"] = &user
			service := newServiceWithDependencies(store, hasher, testServerID)
			if _, err := service.RecordAuthenticationResult(context.Background(), test.input); !errors.Is(err, test.wantErr) {
				t.Fatalf("RecordAuthenticationResult() error = %v, want %v", err, test.wantErr)
			}
			if store.upsertCount != 0 {
				t.Fatalf("rejected authentication reached store: %d", store.upsertCount)
			}
		})
	}
}

func TestResolvePrincipalChecksMappingAndLiveUserState(t *testing.T) {
	now := time.Date(2026, 8, 22, 8, 0, 0, 0, time.UTC)
	hasher, _ := tokenhash.New("fixture-root-key", tokenHashPurpose)
	digest, _ := hasher.Sum(testAccessToken)
	userID := "user-1"
	store := newFakeStore()
	store.mapping = &models.EmbyAccessToken{
		ID: "mapping-1", ServerID: testServerID, TokenHash: digest,
		EmbyUserID: "emby-user-1", UserID: &userID, DeviceID: "device-1",
		ClientName: "Infuse", LastSeenAt: now.Add(-10 * time.Minute),
	}
	store.usersByID[userID] = &models.User{
		ID: userID, EmbyID: "emby-user-1", IsActive: true,
	}
	service := newServiceWithDependencies(store, hasher, testServerID)
	service.now = func() time.Time { return now }

	principal, err := service.ResolvePrincipal(context.Background(), testAccessToken)
	if err != nil {
		t.Fatalf("ResolvePrincipal() error = %v", err)
	}
	if principal.User.ID != userID || principal.MappingID != "mapping-1" || principal.DeviceID != "device-1" {
		t.Fatalf("ResolvePrincipal() = %+v", principal)
	}
	if store.touchCount != 1 || !store.touchedAt.Equal(now) {
		t.Fatalf("TouchLastSeen() count=%d at=%v", store.touchCount, store.touchedAt)
	}

	store.mapping.LastSeenAt = now.Add(-time.Minute)
	store.touchCount = 0
	if _, err := service.ResolvePrincipal(context.Background(), testAccessToken); err != nil {
		t.Fatalf("ResolvePrincipal(recent) error = %v", err)
	}
	if store.touchCount != 0 {
		t.Fatalf("recent mapping was touched: %d", store.touchCount)
	}
}

func TestResolvePrincipalRejectsRevokedExpiredAndIdentityMismatch(t *testing.T) {
	now := time.Date(2026, 8, 22, 8, 0, 0, 0, time.UTC)
	hasher, _ := tokenhash.New("fixture-root-key", tokenHashPurpose)
	digest, _ := hasher.Sum(testAccessToken)
	userID := "user-1"
	revokedAt := now.Add(-time.Minute)
	expiredAt := now.Add(-time.Minute)
	tests := []struct {
		name    string
		mapping models.EmbyAccessToken
		user    models.User
		wantErr error
	}{
		{
			name: "revoked",
			mapping: models.EmbyAccessToken{ID: "mapping-1", ServerID: testServerID, TokenHash: digest,
				EmbyUserID: "emby-user-1", UserID: &userID, RevokedAt: &revokedAt},
			user:    models.User{ID: userID, EmbyID: "emby-user-1", IsActive: true},
			wantErr: ErrTokenRevoked,
		},
		{
			name: "expired",
			mapping: models.EmbyAccessToken{ID: "mapping-1", ServerID: testServerID, TokenHash: digest,
				EmbyUserID: "emby-user-1", UserID: &userID},
			user:    models.User{ID: userID, EmbyID: "emby-user-1", IsActive: true, ExpiresAt: &expiredAt},
			wantErr: ErrUserExpired,
		},
		{
			name: "emby id mismatch",
			mapping: models.EmbyAccessToken{ID: "mapping-1", ServerID: testServerID, TokenHash: digest,
				EmbyUserID: "emby-user-1", UserID: &userID},
			user:    models.User{ID: userID, EmbyID: "other-user", IsActive: true},
			wantErr: ErrIdentityMismatch,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newFakeStore()
			mapping := test.mapping
			user := test.user
			store.mapping = &mapping
			store.usersByID[userID] = &user
			service := newServiceWithDependencies(store, hasher, testServerID)
			service.now = func() time.Time { return now }
			if _, err := service.ResolvePrincipal(context.Background(), testAccessToken); !errors.Is(err, test.wantErr) {
				t.Fatalf("ResolvePrincipal() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestServiceRevokeScopesUseBoundedAuditMetadata(t *testing.T) {
	store := newFakeStore()
	hasher, _ := tokenhash.New("fixture-root-key", tokenHashPurpose)
	service := newServiceWithDependencies(store, hasher, testServerID)
	now := time.Date(2026, 8, 22, 8, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	if count, err := service.RevokeToken(context.Background(), "mapping-1", RevokeReasonManualTokenLogout, "admin-1"); err != nil || count != 1 {
		t.Fatalf("RevokeToken() count=%d error=%v", count, err)
	}
	if count, err := service.RevokeDevice(context.Background(), "user-1", "device-1", RevokeReasonManualDeviceLogout, "admin-1"); err != nil || count != 2 {
		t.Fatalf("RevokeDevice() count=%d error=%v", count, err)
	}
	if count, err := service.RevokeUserTokens(context.Background(), "user-1", RevokeReasonManualUserLogout, "admin-1"); err != nil || count != 3 {
		t.Fatalf("RevokeUserTokens() count=%d error=%v", count, err)
	}
	if store.lastRevoke.ServerID != testServerID || store.lastRevoke.RevokedBy != "admin-1" || !store.lastRevoke.At.Equal(now) {
		t.Fatalf("revoke input = %+v", store.lastRevoke)
	}
}

func TestServiceRejectsUnboundedRevocationInput(t *testing.T) {
	store := newFakeStore()
	hasher, _ := tokenhash.New("fixture-root-key", tokenHashPurpose)
	service := newServiceWithDependencies(store, hasher, testServerID)
	if _, err := service.RevokeUserTokens(context.Background(), "user-1", RevokeReason("arbitrary"), "admin-1"); !errors.Is(err, ErrRevokeReasonInvalid) {
		t.Fatalf("RevokeUserTokens(arbitrary reason) error = %v", err)
	}
	if _, err := service.RevokeDevice(context.Background(), "user-1", "", RevokeReasonManualDeviceLogout, "admin-1"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("RevokeDevice(empty device) error = %v", err)
	}
}

type fakeStore struct {
	usersByEmbyID map[string]*models.User
	usersByID     map[string]*models.User
	mapping       *models.EmbyAccessToken
	upsertInput   upsertMappingInput
	upsertCount   int
	touchCount    int
	touchedAt     time.Time
	lastRevoke    revokeInput
}

func newFakeStore() *fakeStore {
	return &fakeStore{usersByEmbyID: map[string]*models.User{}, usersByID: map[string]*models.User{}}
}

func (store *fakeStore) FindUserByEmbyID(_ context.Context, embyUserID string) (*models.User, error) {
	user, ok := store.usersByEmbyID[embyUserID]
	if !ok {
		return nil, ErrUserNotFound
	}
	copy := *user
	return &copy, nil
}

func (store *fakeStore) FindUserByID(_ context.Context, userID string) (*models.User, error) {
	user, ok := store.usersByID[userID]
	if !ok {
		return nil, ErrUserNotFound
	}
	copy := *user
	return &copy, nil
}

func (store *fakeStore) UpsertMapping(_ context.Context, input upsertMappingInput) (*models.EmbyAccessToken, error) {
	store.upsertCount++
	store.upsertInput = input
	userID := input.UserID
	mapping := &models.EmbyAccessToken{
		ID: "mapping-1", ServerID: input.ServerID, TokenHash: append([]byte(nil), input.TokenHash...),
		EmbyUserID: input.EmbyUserID, UserID: &userID, DeviceID: input.DeviceID,
		ClientName: input.ClientName, LastSeenAt: input.At,
	}
	store.mapping = mapping
	return mapping, nil
}

func (store *fakeStore) FindMapping(_ context.Context, _ string, _ []byte) (*models.EmbyAccessToken, error) {
	if store.mapping == nil {
		return nil, ErrTokenNotFound
	}
	copy := *store.mapping
	return &copy, nil
}

func (store *fakeStore) TouchLastSeen(_ context.Context, _ string, at, _ time.Time) error {
	store.touchCount++
	store.touchedAt = at
	return nil
}

func (store *fakeStore) RevokeToken(_ context.Context, input revokeInput) (int64, error) {
	store.lastRevoke = input
	return 1, nil
}

func (store *fakeStore) RevokeDevice(_ context.Context, input revokeInput) (int64, error) {
	store.lastRevoke = input
	return 2, nil
}

func (store *fakeStore) RevokeUser(_ context.Context, input revokeInput) (int64, error) {
	store.lastRevoke = input
	return 3, nil
}
