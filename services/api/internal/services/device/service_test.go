package device

import (
	"context"
	"errors"
	"sort"
	"testing"

	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	embytokenpkg "github.com/konghang/ember/backend/internal/services/embytoken"
	"gorm.io/gorm"
)

func TestAddClientToBlacklistCreatesNormalizedEntryAndRecordsAction(t *testing.T) {
	var created *models.ClientBlacklist
	var recorded *models.DeviceAction
	service := &DeviceService{
		findClientBlacklist: func(normalized string) (*models.ClientBlacklist, error) {
			if normalized != "infuse" {
				t.Fatalf("expected normalized client name infuse, got %q", normalized)
			}
			return nil, gorm.ErrRecordNotFound
		},
		createClientBlacklist: func(blacklist *models.ClientBlacklist) error {
			copy := *blacklist
			created = &copy
			return nil
		},
		updateClientBlacklist: func(blacklist *models.ClientBlacklist) error {
			t.Fatalf("updateClientBlacklist must not run for a new blacklist")
			return nil
		},
		recordDeviceActionFn: func(action models.DeviceAction) error {
			copy := action
			recorded = &copy
			return nil
		},
	}

	err := service.AddClientToBlacklist(" Infuse 7.8.1 ", "  blocked client  ", "admin_1")

	if err != nil {
		t.Fatalf("expected blacklist create success, got %v", err)
	}
	if created == nil || created.ClientName != "Infuse 7.8.1" || created.NormalizedClientName != "infuse" || created.Reason != "blocked client" {
		t.Fatalf("unexpected created blacklist: %+v", created)
	}
	if recorded == nil || recorded.ClientName != "Infuse 7.8.1" || recorded.Action != "blacklist" || recorded.Note != "blocked client" {
		t.Fatalf("unexpected recorded action: %+v", recorded)
	}
	if recorded.OperatorID == nil || *recorded.OperatorID != "admin_1" {
		t.Fatalf("expected operator admin_1, got %+v", recorded.OperatorID)
	}
}

func TestAddClientToBlacklistUpdatesExistingEntry(t *testing.T) {
	var updated *models.ClientBlacklist
	service := &DeviceService{
		findClientBlacklist: func(normalized string) (*models.ClientBlacklist, error) {
			return &models.ClientBlacklist{
				ID:                   "blacklist_1",
				ClientName:           "Infuse",
				NormalizedClientName: normalized,
				Reason:               "old",
			}, nil
		},
		createClientBlacklist: func(blacklist *models.ClientBlacklist) error {
			t.Fatalf("createClientBlacklist must not run for existing blacklist")
			return nil
		},
		updateClientBlacklist: func(blacklist *models.ClientBlacklist) error {
			copy := *blacklist
			updated = &copy
			return nil
		},
		recordDeviceActionFn: func(action models.DeviceAction) error {
			return nil
		},
	}

	err := service.AddClientToBlacklist("Infuse Pro", "new reason", "admin_1")

	if err != nil {
		t.Fatalf("expected blacklist update success, got %v", err)
	}
	if updated == nil || updated.ID != "blacklist_1" || updated.ClientName != "Infuse Pro" ||
		updated.NormalizedClientName != "infuse pro" || updated.Reason != "new reason" {
		t.Fatalf("unexpected updated blacklist: %+v", updated)
	}
}

func TestAddClientToBlacklistRejectsBlankClientBeforeStore(t *testing.T) {
	service := &DeviceService{
		findClientBlacklist: func(normalized string) (*models.ClientBlacklist, error) {
			t.Fatalf("store lookup must not run for blank client")
			return nil, nil
		},
	}

	err := service.AddClientToBlacklist("  ", "reason", "admin_1")
	if !errors.Is(err, ErrDeviceClientNameRequired) {
		t.Fatalf("expected ErrDeviceClientNameRequired, got %v", err)
	}
}

func TestAddClientToBlacklistMapsStoreFailures(t *testing.T) {
	service := &DeviceService{
		findClientBlacklist: func(normalized string) (*models.ClientBlacklist, error) {
			return nil, errors.New("database unavailable")
		},
	}

	err := service.AddClientToBlacklist("Infuse", "reason", "admin_1")
	if err == nil || err.Error() != "添加黑名单失败" {
		t.Fatalf("expected masked store failure, got %v", err)
	}
}

func TestRemoveClientFromBlacklistDeletesExistingEntryAndRecordsAction(t *testing.T) {
	var deleted *models.ClientBlacklist
	var recorded *models.DeviceAction
	service := &DeviceService{
		findClientBlacklist: func(normalized string) (*models.ClientBlacklist, error) {
			if normalized != "infuse" {
				t.Fatalf("expected normalized client name infuse, got %q", normalized)
			}
			return &models.ClientBlacklist{
				ID:                   "blacklist_1",
				ClientName:           "Infuse 7.8.1",
				NormalizedClientName: normalized,
				Reason:               "blocked",
			}, nil
		},
		deleteClientBlacklist: func(blacklist *models.ClientBlacklist) error {
			copy := *blacklist
			deleted = &copy
			return nil
		},
		recordDeviceActionFn: func(action models.DeviceAction) error {
			copy := action
			recorded = &copy
			return nil
		},
	}

	err := service.RemoveClientFromBlacklist(" Infuse 7.8.1 ", "admin_1")

	if err != nil {
		t.Fatalf("expected blacklist remove success, got %v", err)
	}
	if deleted == nil || deleted.ID != "blacklist_1" || deleted.ClientName != "Infuse 7.8.1" {
		t.Fatalf("unexpected deleted blacklist: %+v", deleted)
	}
	if recorded == nil || recorded.ClientName != "Infuse 7.8.1" || recorded.Action != "unblacklist" || recorded.Note != "" {
		t.Fatalf("unexpected recorded action: %+v", recorded)
	}
	if recorded.OperatorID == nil || *recorded.OperatorID != "admin_1" {
		t.Fatalf("expected operator admin_1, got %+v", recorded.OperatorID)
	}
}

func TestRemoveClientFromBlacklistReturnsNotFound(t *testing.T) {
	service := &DeviceService{
		findClientBlacklist: func(normalized string) (*models.ClientBlacklist, error) {
			return nil, gorm.ErrRecordNotFound
		},
		deleteClientBlacklist: func(blacklist *models.ClientBlacklist) error {
			t.Fatalf("deleteClientBlacklist must not run when blacklist is missing")
			return nil
		},
	}

	err := service.RemoveClientFromBlacklist("Infuse", "admin_1")
	if !errors.Is(err, ErrClientBlacklistNotFound) {
		t.Fatalf("expected ErrClientBlacklistNotFound, got %v", err)
	}
}

func TestRemoveClientFromBlacklistRejectsBlankClientBeforeStore(t *testing.T) {
	service := &DeviceService{
		findClientBlacklist: func(normalized string) (*models.ClientBlacklist, error) {
			t.Fatalf("store lookup must not run for blank client")
			return nil, nil
		},
		deleteClientBlacklist: func(blacklist *models.ClientBlacklist) error {
			t.Fatalf("deleteClientBlacklist must not run for blank client")
			return nil
		},
	}

	err := service.RemoveClientFromBlacklist("  ", "admin_1")
	if !errors.Is(err, ErrDeviceClientNameRequired) {
		t.Fatalf("expected ErrDeviceClientNameRequired, got %v", err)
	}
}

func TestRemoveClientFromBlacklistMapsDeleteFailures(t *testing.T) {
	service := &DeviceService{
		findClientBlacklist: func(normalized string) (*models.ClientBlacklist, error) {
			return &models.ClientBlacklist{
				ID:                   "blacklist_1",
				ClientName:           "Infuse",
				NormalizedClientName: normalized,
			}, nil
		},
		deleteClientBlacklist: func(blacklist *models.ClientBlacklist) error {
			return errors.New("database unavailable")
		},
	}

	err := service.RemoveClientFromBlacklist("Infuse", "admin_1")
	if err == nil || err.Error() != "移除黑名单失败" {
		t.Fatalf("expected masked delete failure, got %v", err)
	}
}

func TestLogoutDeviceRecordsDeviceContextAfterLogout(t *testing.T) {
	var loggedOut string
	var recorded *models.DeviceAction
	service := &DeviceService{
		buildDeviceItemsFn: func() ([]DeviceItem, error) {
			return []DeviceItem{
				{DeviceID: "device_1", UserID: "user_1", ClientName: "Infuse"},
				{DeviceID: "device_2", UserID: "user_2", ClientName: "Kodi"},
			}, nil
		},
		logoutDeviceFn: func(deviceID string) error {
			loggedOut = deviceID
			return nil
		},
		revokeDeviceTokensFn: func(context.Context, string, string, embytokenpkg.RevokeReason, string) (int64, error) {
			return 0, nil
		},
		recordDeviceActionFn: func(action models.DeviceAction) error {
			copy := action
			recorded = &copy
			return nil
		},
	}

	err := service.LogoutDevice(" device_1 ", "admin_1")

	if err != nil {
		t.Fatalf("expected logout success, got %v", err)
	}
	if loggedOut != "device_1" {
		t.Fatalf("expected logout device_1, got %q", loggedOut)
	}
	if recorded == nil || recorded.DeviceID != "device_1" || recorded.UserID != "user_1" ||
		recorded.ClientName != "Infuse" || recorded.Action != "logout" || recorded.Note != "manual" {
		t.Fatalf("unexpected recorded action: %+v", recorded)
	}
	if recorded.OperatorID == nil || *recorded.OperatorID != "admin_1" {
		t.Fatalf("expected operator admin_1, got %+v", recorded.OperatorID)
	}
}

func TestLogoutDeviceRejectsBlankDeviceBeforeDependencies(t *testing.T) {
	service := &DeviceService{
		buildDeviceItemsFn: func() ([]DeviceItem, error) {
			t.Fatalf("buildDeviceItemsFn must not run for blank device")
			return nil, nil
		},
		logoutDeviceFn: func(deviceID string) error {
			t.Fatalf("logoutDeviceFn must not run for blank device")
			return nil
		},
		revokeDeviceTokensFn: func(context.Context, string, string, embytokenpkg.RevokeReason, string) (int64, error) {
			return 1, nil
		},
		recordDeviceActionFn: func(action models.DeviceAction) error {
			t.Fatalf("recordDeviceActionFn must not run for blank device")
			return nil
		},
	}

	err := service.LogoutDevice("  ", "admin_1")
	if !errors.Is(err, ErrDeviceIDRequired) {
		t.Fatalf("expected ErrDeviceIDRequired, got %v", err)
	}
}

func TestLogoutDeviceReturnsLogoutFailureWithoutRecordingAction(t *testing.T) {
	service := &DeviceService{
		buildDeviceItemsFn: func() ([]DeviceItem, error) {
			return []DeviceItem{
				{DeviceID: "device_1", UserID: "user_1", ClientName: "Infuse"},
			}, nil
		},
		logoutDeviceFn: func(deviceID string) error {
			return errors.New("emby timeout")
		},
		revokeDeviceTokensFn: func(context.Context, string, string, embytokenpkg.RevokeReason, string) (int64, error) {
			return 0, nil
		},
		recordDeviceActionFn: func(action models.DeviceAction) error {
			t.Fatalf("recordDeviceActionFn must not run when logout fails")
			return nil
		},
	}

	err := service.LogoutDevice("device_1", "admin_1")
	if err == nil || err.Error() != "emby timeout" {
		t.Fatalf("expected logout failure, got %v", err)
	}
}

func TestLogoutDeviceRevokesLocalTokensBeforeRemoteLogout(t *testing.T) {
	var order []string
	service := &DeviceService{
		buildDeviceItemsFn: func() ([]DeviceItem, error) {
			return []DeviceItem{{DeviceID: "device_1", UserID: "user_1", ClientName: "Infuse"}}, nil
		},
		revokeDeviceTokensFn: func(_ context.Context, userID, deviceID string, reason embytokenpkg.RevokeReason, actor string) (int64, error) {
			order = append(order, "revoke")
			if userID != "user_1" || deviceID != "device_1" || reason != embytokenpkg.RevokeReasonManualDeviceLogout || actor != "admin_1" {
				t.Fatalf("revoke input user=%s device=%s reason=%s actor=%s", userID, deviceID, reason, actor)
			}
			return 1, nil
		},
		logoutDeviceFn: func(deviceID string) error {
			order = append(order, "remote")
			return nil
		},
		recordDeviceActionFn: func(models.DeviceAction) error { return nil },
	}
	if err := service.LogoutDeviceWithContext(context.Background(), " device_1 ", "admin_1"); err != nil {
		t.Fatalf("LogoutDeviceWithContext() error = %v", err)
	}
	if len(order) != 2 || order[0] != "revoke" || order[1] != "remote" {
		t.Fatalf("operation order = %#v", order)
	}
}

func TestLogoutDeviceStopsBeforeRemoteWhenOwnerOrRevocationFails(t *testing.T) {
	tests := []struct {
		name       string
		items      []DeviceItem
		buildErr   error
		revokeErr  error
		wantErr    error
		wantRevoke int
	}{
		{name: "device owner missing", items: []DeviceItem{{DeviceID: "device_1"}}, wantErr: ErrDeviceOwnerUnavailable},
		{name: "device owner lookup fails", buildErr: errors.New("lookup failed"), wantErr: ErrDeviceOwnerUnavailable},
		{name: "local revocation fails", items: []DeviceItem{{DeviceID: "device_1", UserID: "user_1"}}, revokeErr: errors.New("database failed"), wantErr: ErrDeviceTokenRevocation, wantRevoke: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			revokeCalls := 0
			remoteCalls := 0
			service := &DeviceService{
				buildDeviceItemsFn: func() ([]DeviceItem, error) { return test.items, test.buildErr },
				revokeDeviceTokensFn: func(context.Context, string, string, embytokenpkg.RevokeReason, string) (int64, error) {
					revokeCalls++
					return 0, test.revokeErr
				},
				logoutDeviceFn:       func(string) error { remoteCalls++; return nil },
				recordDeviceActionFn: func(models.DeviceAction) error { return nil },
			}
			if err := service.LogoutDeviceWithContext(context.Background(), "device_1", "admin_1"); !errors.Is(err, test.wantErr) {
				t.Fatalf("LogoutDeviceWithContext() error = %v, want %v", err, test.wantErr)
			}
			if revokeCalls != test.wantRevoke || remoteCalls != 0 {
				t.Fatalf("calls revoke=%d remote=%d", revokeCalls, remoteCalls)
			}
		})
	}
}

func TestLogoutDeviceKeepsLocalRevocationWhenRemoteLogoutFails(t *testing.T) {
	revoked := false
	service := &DeviceService{
		buildDeviceItemsFn: func() ([]DeviceItem, error) {
			return []DeviceItem{{DeviceID: "device_1", UserID: "user_1"}}, nil
		},
		revokeDeviceTokensFn: func(context.Context, string, string, embytokenpkg.RevokeReason, string) (int64, error) {
			revoked = true
			return 1, nil
		},
		logoutDeviceFn: func(string) error { return errors.New("emby unavailable") },
		recordDeviceActionFn: func(models.DeviceAction) error {
			t.Fatal("action must not be recorded for remote failure")
			return nil
		},
	}
	if err := service.LogoutDeviceWithContext(context.Background(), "device_1", "admin_1"); err == nil {
		t.Fatal("LogoutDeviceWithContext() error = nil")
	}
	if !revoked {
		t.Fatal("local revocation did not run before remote failure")
	}
}

func TestLogoutBlacklistedDevicesLogsOutUniqueBlacklistedDevicesAndRecordsActions(t *testing.T) {
	var loggedOut []string
	var recorded []models.DeviceAction
	service := &DeviceService{
		buildDeviceItemsFn: func() ([]DeviceItem, error) {
			return []DeviceItem{
				{DeviceID: "device_1", UserID: "user_1", ClientName: "Infuse", IsBlacklisted: true},
				{DeviceID: "device_1", UserID: "user_1", ClientName: "Infuse", IsBlacklisted: true},
				{DeviceID: "device_2", UserID: "user_2", ClientName: "Kodi", IsBlacklisted: false},
				{DeviceID: "", UserID: "user_3", ClientName: "Unknown", IsBlacklisted: true},
				{DeviceID: "device_3", UserID: "user_3", ClientName: "VidHub", IsBlacklisted: true},
			}, nil
		},
		logoutDeviceFn: func(deviceID string) error {
			loggedOut = append(loggedOut, deviceID)
			if deviceID == "device_3" {
				return errors.New("emby timeout")
			}
			return nil
		},
		revokeDeviceTokensFn: func(context.Context, string, string, embytokenpkg.RevokeReason, string) (int64, error) {
			return 1, nil
		},
		recordDeviceActionFn: func(action models.DeviceAction) error {
			recorded = append(recorded, action)
			return nil
		},
	}

	result, err := service.LogoutBlacklistedDevices("admin_1")

	if err != nil {
		t.Fatalf("expected batch logout success, got %v", err)
	}
	sort.Strings(loggedOut)
	if len(loggedOut) != 2 || loggedOut[0] != "device_1" || loggedOut[1] != "device_3" {
		t.Fatalf("unexpected logged out devices: %+v", loggedOut)
	}
	sort.Strings(result.SuccessDeviceIDs)
	if len(result.SuccessDeviceIDs) != 1 || result.SuccessDeviceIDs[0] != "device_1" {
		t.Fatalf("unexpected success devices: %+v", result.SuccessDeviceIDs)
	}
	if len(result.FailedDeviceIDs) != 1 || result.FailedDeviceIDs[0].DeviceID != "device_3" ||
		result.FailedDeviceIDs[0].Error != "emby timeout" {
		t.Fatalf("unexpected failed devices: %+v", result.FailedDeviceIDs)
	}
	if len(recorded) != 1 {
		t.Fatalf("expected one recorded action, got %+v", recorded)
	}
	action := recorded[0]
	if action.DeviceID != "device_1" || action.UserID != "user_1" || action.ClientName != "Infuse" ||
		action.Action != "logout" || action.Note != "blacklist" {
		t.Fatalf("unexpected recorded action: %+v", action)
	}
	if action.OperatorID == nil || *action.OperatorID != "admin_1" {
		t.Fatalf("expected operator admin_1, got %+v", action.OperatorID)
	}
}

func TestLogoutBlacklistedDevicesReturnsBuildFailure(t *testing.T) {
	service := &DeviceService{
		buildDeviceItemsFn: func() ([]DeviceItem, error) {
			return nil, errors.New("emby unavailable")
		},
		logoutDeviceFn: func(deviceID string) error {
			t.Fatalf("logoutDeviceFn must not run when device item build fails")
			return nil
		},
	}

	result, err := service.LogoutBlacklistedDevices("admin_1")
	if err == nil || err.Error() != "emby unavailable" {
		t.Fatalf("expected build failure, got result=%+v err=%v", result, err)
	}
	if result != nil {
		t.Fatalf("expected nil result on build failure, got %+v", result)
	}
}

func TestLogoutBlacklistedDevicesSkipsRemoteWhenLocalRevocationFails(t *testing.T) {
	var remote []string
	service := &DeviceService{
		buildDeviceItemsFn: func() ([]DeviceItem, error) {
			return []DeviceItem{
				{DeviceID: "device_1", UserID: "user_1", IsBlacklisted: true},
				{DeviceID: "device_2", UserID: "user_2", IsBlacklisted: true},
			}, nil
		},
		revokeDeviceTokensFn: func(_ context.Context, userID, deviceID string, reason embytokenpkg.RevokeReason, actor string) (int64, error) {
			if reason != embytokenpkg.RevokeReasonSecurityRevoke || actor != "admin_1" {
				t.Fatalf("reason=%s actor=%s", reason, actor)
			}
			if deviceID == "device_1" {
				return 0, errors.New("revoke failed")
			}
			return 1, nil
		},
		logoutDeviceFn: func(deviceID string) error {
			remote = append(remote, deviceID)
			return nil
		},
		recordDeviceActionFn: func(models.DeviceAction) error { return nil },
	}
	result, err := service.LogoutBlacklistedDevicesWithContext(context.Background(), "admin_1")
	if err != nil {
		t.Fatalf("LogoutBlacklistedDevicesWithContext() error = %v", err)
	}
	if len(remote) != 1 || remote[0] != "device_2" {
		t.Fatalf("remote calls = %#v", remote)
	}
	if len(result.SuccessDeviceIDs) != 1 || result.SuccessDeviceIDs[0] != "device_2" ||
		len(result.FailedDeviceIDs) != 1 || result.FailedDeviceIDs[0].DeviceID != "device_1" ||
		result.FailedDeviceIDs[0].Error != ErrDeviceTokenRevocation.Error() {
		t.Fatalf("result = %+v", result)
	}
}

func TestBuildDeviceItemsFromSourcesMergesDeviceSessionUserAndBlacklist(t *testing.T) {
	var userLookupIDs []string
	service := &DeviceService{
		getEmbyDevicesFn: func() ([]embyint.EmbyDevice, error) {
			return []embyint.EmbyDevice{
				{
					ID:               "device_1",
					Name:             "Apple TV",
					AppName:          "Infuse 7.8.1",
					AppVersion:       "7.8.1",
					LastUserName:     "Emby Alice",
					LastUserID:       "emby_user_1",
					LastActivityDate: "2026-06-16T10:00:00Z",
				},
				{
					ID:          "device_2",
					Name:        "Bedroom",
					DateCreated: "2026-06-14T10:00:00Z",
				},
				{ID: "   ", Name: "Ignored"},
			}, nil
		},
		getEmbySessionsFn: func() ([]embyint.EmbySession, error) {
			return []embyint.EmbySession{
				{
					DeviceID:           "device_1",
					DeviceName:         "Living Room",
					Client:             "Infuse",
					UserID:             "emby_user_1",
					UserName:           "Session Alice",
					ApplicationVersion: "8.0.0",
					LastActivityDate:   "2026-06-17T10:00:00Z",
					RemoteEndPoint:     "10.0.0.8",
					NowPlayingItem:     &embyint.EmbyNowPlayingItem{},
				},
				{DeviceID: "  ", DeviceName: "Ignored"},
			}, nil
		},
		listClientBlacklists: func() ([]models.ClientBlacklist, error) {
			return []models.ClientBlacklist{
				{NormalizedClientName: "infuse", Reason: "blocked player"},
			}, nil
		},
		listUsersByEmbyIDs: func(embyIDs []string) ([]models.User, error) {
			userLookupIDs = append(userLookupIDs, embyIDs...)
			return []models.User{
				{ID: "user_1", Username: "alice", EmbyID: "emby_user_1"},
			}, nil
		},
	}

	items, err := service.buildDeviceItemsFromSources()

	if err != nil {
		t.Fatalf("expected build success, got %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two device items, got %+v", items)
	}
	if len(userLookupIDs) != 1 || userLookupIDs[0] != "emby_user_1" {
		t.Fatalf("unexpected user lookup ids: %+v", userLookupIDs)
	}

	active := items[0]
	if active.DeviceID != "device_1" || active.DeviceName != "Living Room" || active.ClientName != "Infuse" {
		t.Fatalf("unexpected active item identity: %+v", active)
	}
	if !active.IsActive || !active.IsBlacklisted || active.BlacklistReason != "blocked player" {
		t.Fatalf("unexpected active item flags: %+v", active)
	}
	if active.UserID != "user_1" || active.UserName != "Session Alice" || active.EmbyUserID != "emby_user_1" {
		t.Fatalf("unexpected active item user mapping: %+v", active)
	}
	if active.ApplicationVersion != "8.0.0" || active.RemoteEndpoint != "10.0.0.8" ||
		active.LastActivityDate != "2026-06-17T10:00:00Z" {
		t.Fatalf("unexpected active item session fields: %+v", active)
	}

	inactive := items[1]
	if inactive.DeviceID != "device_2" || inactive.DeviceName != "Bedroom" || inactive.ClientName != "Unknown" {
		t.Fatalf("unexpected inactive item defaults: %+v", inactive)
	}
	if inactive.IsActive || inactive.IsBlacklisted {
		t.Fatalf("unexpected inactive item flags: %+v", inactive)
	}
}

func TestBuildDeviceItemsFromSourcesReturnsFailureWhenDevicesAndSessionsFail(t *testing.T) {
	service := &DeviceService{
		getEmbyDevicesFn: func() ([]embyint.EmbyDevice, error) {
			return nil, errors.New("devices unavailable")
		},
		getEmbySessionsFn: func() ([]embyint.EmbySession, error) {
			return nil, errors.New("sessions unavailable")
		},
		listClientBlacklists: func() ([]models.ClientBlacklist, error) {
			t.Fatalf("listClientBlacklists must not run when Emby sources both fail")
			return nil, nil
		},
	}

	items, err := service.buildDeviceItemsFromSources()
	if err == nil || err.Error() != "获取设备列表失败" {
		t.Fatalf("expected device list failure, got items=%+v err=%v", items, err)
	}
	if items != nil {
		t.Fatalf("expected nil items on failure, got %+v", items)
	}
}
