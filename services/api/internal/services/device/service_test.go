package device

import (
	"errors"
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
