package device

import (
	"errors"
	"sort"
	"testing"

	"github.com/konghang/ember/backend/internal/models"
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
