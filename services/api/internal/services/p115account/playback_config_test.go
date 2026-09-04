package p115account

import (
	"context"
	"errors"
	"testing"
	"time"

	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/models"
)

type fakeDirectoryResolver struct {
	directory  *p115integration.Directory
	err        error
	calls      int
	credential p115integration.Credential
	query      p115integration.DirectoryPathQuery
}

func (r *fakeDirectoryResolver) ResolveDirectoryByPath(
	_ context.Context,
	credential p115integration.Credential,
	query p115integration.DirectoryPathQuery,
) (*p115integration.Directory, error) {
	r.calls++
	r.credential = credential
	r.query = query
	return r.directory, r.err
}

func TestServiceUpdatePlaybackConfigResolvesAndPersistsAtomicFields(t *testing.T) {
	updatedAt := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	store := &fakeAccountStore{accounts: map[string]*models.P115Account{
		"playback": {
			ID: "playback", Role: models.P115AccountRolePlayback,
			CookieCiphertext: stringPointer("encrypted:cookie"), AppType: stringPointer("web"), UserAgent: stringPointer("provider-agent"),
			Status: models.P115AccountStatusActive, UpdatedAt: updatedAt,
		},
	}}
	resolver := &fakeDirectoryResolver{directory: &p115integration.Directory{ID: "200", Path: "/Ember/Playback"}}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})
	service.directoryResolver = resolver

	result, err := service.UpdatePlaybackConfig(context.Background(), "playback", PlaybackConfigInput{
		TargetParentPath:     " Ember/Playback ",
		MaxConcurrentStreams: 3,
	})
	if err != nil {
		t.Fatalf("UpdatePlaybackConfig() error = %v", err)
	}
	if resolver.calls != 1 || resolver.credential.Cookie != "cookie" || resolver.query.RootID != "0" || resolver.query.RelativePath != "Ember/Playback" {
		t.Fatalf("resolver call = credential=%+v query=%+v calls=%d", resolver.credential, resolver.query, resolver.calls)
	}
	if store.playbackConfigID != "playback" || store.playbackConfigExpectedCiphertext != "encrypted:cookie" || !store.playbackConfigExpectedUpdatedAt.Equal(updatedAt) {
		t.Fatalf("store guard = id=%q ciphertext=%q updatedAt=%s", store.playbackConfigID, store.playbackConfigExpectedCiphertext, store.playbackConfigExpectedUpdatedAt)
	}
	if store.playbackConfigPath != "/Ember/Playback" || store.playbackConfigTargetID != "200" || store.playbackConfigMax != 3 {
		t.Fatalf("stored config = path=%q target=%q max=%d", store.playbackConfigPath, store.playbackConfigTargetID, store.playbackConfigMax)
	}
	if result.TargetParentPath == nil || *result.TargetParentPath != "/Ember/Playback" || result.MaxConcurrentStreams == nil || *result.MaxConcurrentStreams != 3 {
		t.Fatalf("result = %+v", result)
	}
}

func TestServiceUpdatePlaybackConfigFailsClosed(t *testing.T) {
	resolver := &fakeDirectoryResolver{directory: &p115integration.Directory{ID: "200", Path: "/Playback"}}
	tests := []struct {
		name    string
		account models.P115Account
		input   PlaybackConfigInput
		wantErr error
	}{
		{name: "source", account: models.P115Account{ID: "account", Role: models.P115AccountRoleSource, Status: models.P115AccountStatusActive}, input: PlaybackConfigInput{TargetParentPath: "/Playback", MaxConcurrentStreams: 1}, wantErr: ErrPlaybackConfigOnly},
		{name: "personal", account: models.P115Account{ID: "account", Role: models.P115AccountRolePlayback, OwnerUserID: stringPointer("user-1"), Status: models.P115AccountStatusActive}, input: PlaybackConfigInput{TargetParentPath: "/Playback", MaxConcurrentStreams: 1}, wantErr: ErrAccountNotFound},
		{name: "pending", account: models.P115Account{ID: "account", Role: models.P115AccountRolePlayback, Status: models.P115AccountStatusPending}, input: PlaybackConfigInput{TargetParentPath: "/Playback", MaxConcurrentStreams: 1}, wantErr: ErrAccountUnavailable},
		{name: "missing path", account: models.P115Account{ID: "account", Role: models.P115AccountRolePlayback, Status: models.P115AccountStatusActive}, input: PlaybackConfigInput{MaxConcurrentStreams: 1}, wantErr: ErrTargetParentPathRequired},
		{name: "zero max", account: models.P115Account{ID: "account", Role: models.P115AccountRolePlayback, Status: models.P115AccountStatusActive}, input: PlaybackConfigInput{TargetParentPath: "/Playback"}, wantErr: ErrMaxConcurrentStreamsInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeAccountStore{accounts: map[string]*models.P115Account{"account": &tt.account}}
			service := newServiceWithDependencies(store, fakeCredentialCipher{})
			service.directoryResolver = resolver
			_, err := service.UpdatePlaybackConfig(context.Background(), "account", tt.input)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("UpdatePlaybackConfig() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestServiceUpdatePlaybackConfigDoesNotWriteResolvedDirectoryAfterAccountChange(t *testing.T) {
	store := &fakeAccountStore{
		accounts: map[string]*models.P115Account{
			"playback": {
				ID: "playback", Role: models.P115AccountRolePlayback,
				CookieCiphertext: stringPointer("encrypted:cookie"), AppType: stringPointer("web"), UserAgent: stringPointer("provider-agent"),
				Status: models.P115AccountStatusActive, UpdatedAt: time.Now().UTC(),
			},
		},
		playbackConfigErr: ErrRuntimeStateChanged,
	}
	service := newServiceWithDependencies(store, fakeCredentialCipher{})
	service.directoryResolver = &fakeDirectoryResolver{directory: &p115integration.Directory{ID: "200", Path: "/Playback"}}

	_, err := service.UpdatePlaybackConfig(context.Background(), "playback", PlaybackConfigInput{
		TargetParentPath:     "/Playback",
		MaxConcurrentStreams: 2,
	})
	if !errors.Is(err, ErrRuntimeStateChanged) {
		t.Fatalf("UpdatePlaybackConfig() error = %v, want ErrRuntimeStateChanged", err)
	}
	if store.accounts["playback"].TargetParentPath != nil || store.accounts["playback"].MaxConcurrentStreams != nil {
		t.Fatalf("stale directory was persisted: %+v", store.accounts["playback"])
	}
}
