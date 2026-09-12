package directplay

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/services/p115account"
)

// TestIntegrationAccountConfigVersionSeparatesHealthAndControls exercises the
// production SQL guards across admission, concurrent health and control writes.
func TestIntegrationAccountConfigVersionSeparatesHealthAndControls(t *testing.T) {
	database := newDirectPlayIntegrationDatabase(t)
	accounts := seedDirectPlayAccounts(t, database)
	ctx := context.Background()
	load := func() models.P115Account {
		t.Helper()
		var account models.P115Account
		if err := database.Where("role = ?", models.P115AccountRolePlayback).First(&account).Error; err != nil {
			t.Fatal(err)
		}
		return account
	}
	before := load()
	route := integrationPlaybackRoute(before)
	first, err := accounts.AcquirePlaybackRoute(ctx, route)
	if err != nil {
		t.Fatal(err)
	}
	if err := accounts.ReportRuntimeHealth(ctx, first, p115account.RuntimeHealthSucceeded); err != nil {
		t.Fatal(err)
	}
	after := load()
	if after.ConfigVersion != before.ConfigVersion || after.LastSucceededAt == nil || after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatal("health must update observation time without advancing configuration")
	}
	second, err := accounts.AcquirePlaybackRoute(ctx, route)
	if err != nil {
		t.Fatalf("health invalidated route: %v", err)
	}
	if err := accounts.ReportRuntimeHealth(ctx, first, p115account.RuntimeHealthCredentialRejected); !errors.Is(err, p115account.ErrRuntimeStateChanged) {
		t.Fatalf("old health result replaced newer success: %v", err)
	}
	if err := accounts.ReportRuntimeHealth(ctx, second, p115account.RuntimeHealthProviderUnavailable); err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.AcquirePlaybackRoute(ctx, route); !errors.Is(err, p115account.ErrAccountCoolingDown) {
		t.Fatalf("current cooldown bypassed: %v", err)
	}
	// Expired cooldown grants one probe without invalidating configuration.
	if err := database.Model(&models.P115Account{}).Where("id = ?", before.ID).
		Update("cooldown_until", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	probe, err := accounts.AcquirePlaybackRoute(ctx, route)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.AcquirePlaybackRoute(ctx, route); !errors.Is(err, p115account.ErrAccountCoolingDown) {
		t.Fatalf("second probe admitted: %v", err)
	}
	if err := accounts.ReportRuntimeHealth(ctx, probe, p115account.RuntimeHealthSucceeded); err != nil {
		t.Fatal(err)
	}
	current, err := accounts.AcquirePlaybackRoute(ctx, route)
	if err != nil {
		t.Fatal(err)
	}
	healthAt := load().UpdatedAt
	for _, enabled := range []bool{false, true} {
		if _, err := accounts.SetEnabled(ctx, before.ID, enabled); err != nil {
			t.Fatal(err)
		}
	}
	after = load()
	if after.ConfigVersion != before.ConfigVersion+2 {
		t.Fatal("off/on must advance configuration twice")
	}
	// Even an identical timestamp cannot make a control change invisible to CAS.
	if err := database.Model(&models.P115Account{}).Where("id = ?", before.ID).Update("updated_at", healthAt).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.AcquirePlaybackRoute(ctx, route); !errors.Is(err, p115account.ErrRuntimeStateChanged) {
		t.Fatalf("old route accepted: %v", err)
	}
	if err := accounts.ReportRuntimeHealth(ctx, current, p115account.RuntimeHealthCredentialRejected); !errors.Is(err, p115account.ErrRuntimeStateChanged) {
		t.Fatalf("old credential generation accepted: %v", err)
	}
}

// integrationPlaybackRoute builds the same non-secret boundary as route metadata.
func integrationPlaybackRoute(account models.P115Account) p115account.PlaybackRoute {
	owner := ""
	if account.OwnerUserID != nil {
		owner = *account.OwnerUserID
	}
	return p115account.PlaybackRoute{AccountID: account.ID, OwnerUserID: owner,
		ProviderUserID: *account.ProviderUserID, TargetParentID: *account.TargetParentID,
		TargetParentPath: *account.TargetParentPath, ConfiguredMaxConcurrentStreams: *account.MaxConcurrentStreams,
		ConfigVersion: account.ConfigVersion, Status: account.Status, CooldownUntil: account.CooldownUntil}
}

// integrationDirectoryValidator allows deterministic concurrent work inside a fake lookup.
type integrationDirectoryValidator struct{ duringLookup func() }

// ValidateCredential returns a fixed identity without contacting 115.
func (integrationDirectoryValidator) ValidateCredential(context.Context, p115integration.Credential) (p115integration.AccountIdentity, error) {
	return p115integration.AccountIdentity{ProviderUserID: "provider-playback"}, nil
}

// ResolveDirectoryByPath runs the competing write before returning a fixed directory.
func (v integrationDirectoryValidator) ResolveDirectoryByPath(context.Context, p115integration.Credential, p115integration.DirectoryPathQuery) (*p115integration.Directory, error) {
	if v.duringLookup != nil {
		v.duringLookup()
	}
	return &p115integration.Directory{ID: "200000002", Path: "/Playback"}, nil
}

// TestIntegrationDirectorySaveIgnoresHealthButRejectsControlChanges checks both
// shared and personal directory saves against real conditional updates.
func TestIntegrationDirectorySaveIgnoresHealthButRejectsControlChanges(t *testing.T) {
	for _, personal := range []bool{false, true} {
		t.Run(map[bool]string{false: "shared", true: "personal"}[personal], func(t *testing.T) {
			database := newDirectPlayIntegrationDatabase(t)
			accounts := seedDirectPlayAccounts(t, database)
			ctx := context.Background()
			var account models.P115Account
			if err := database.Where("role = ?", models.P115AccountRolePlayback).First(&account).Error; err != nil {
				t.Fatal(err)
			}
			owner := "version-owner"
			if personal {
				user := models.User{ID: owner, Username: owner, Email: "version@example.com"}
				if err := database.Create(&user).Error; err != nil {
					t.Fatal(err)
				}
				if err := database.Model(&models.P115Account{}).Where("id = ?", account.ID).Update("owner_user_id", owner).Error; err != nil {
					t.Fatal(err)
				}
				account.OwnerUserID = &owner
			}
			version := account.ConfigVersion
			validator := integrationDirectoryValidator{duringLookup: func() {
				credential, err := accounts.AcquirePlaybackRoute(ctx, integrationPlaybackRoute(account))
				if err != nil {
					t.Fatal(err)
				}
				if err := accounts.ReportRuntimeHealth(ctx, credential, p115account.RuntimeHealthSucceeded); err != nil {
					t.Fatal(err)
				}
			}}
			manager, err := p115account.NewService(database, strings.Repeat("k", 32), validator)
			if err != nil {
				t.Fatal(err)
			}
			save := func() error {
				if personal {
					_, err := manager.UpdatePersonalDirectory(ctx, owner, "/Playback")
					return err
				}
				_, err := manager.UpdatePlaybackConfig(ctx, account.ID, p115account.PlaybackConfigInput{TargetParentPath: "/Playback", MaxConcurrentStreams: 3})
				return err
			}
			if err := save(); err != nil {
				t.Fatalf("health caused a false save conflict: %v", err)
			}
			if err := database.Where("id = ?", account.ID).First(&account).Error; err != nil {
				t.Fatal(err)
			}
			if account.ConfigVersion != version+1 {
				t.Fatal("directory save did not advance configuration")
			}
			validator.duringLookup = func() {
				if personal {
					_, err = accounts.ReplacePersonalCookie(ctx, owner, "UID=100_F1_1700000000; CID=fixture")
				} else {
					_, err = accounts.ReplaceCookie(ctx, account.ID, p115account.ReplaceCookieInput{Cookie: "replacement-cookie", AppType: "ios"})
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			manager, err = p115account.NewService(database, strings.Repeat("k", 32), validator)
			if err != nil {
				t.Fatal(err)
			}
			if err := save(); !errors.Is(err, p115account.ErrRuntimeStateChanged) {
				t.Fatalf("concurrent Cookie replacement was overwritten: %v", err)
			}
		})
	}
}

// TestIntegrationAccountConfigVersionMigration covers upgrading populated tables
// and repeat execution without resetting an existing configuration generation.
func TestIntegrationAccountConfigVersionMigration(t *testing.T) {
	database := newDirectPlayIntegrationDatabase(t)
	seedDirectPlayAccounts(t, database)
	if err := database.Exec("ALTER TABLE p115_accounts DROP COLUMN config_version").Error; err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(directPlayMigrationsDir(t), "20260912_01_p115_account_config_version.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(string(content)).Error; err != nil {
		t.Fatal(err)
	}
	var invalid int64
	if err := database.Model(&models.P115Account{}).Where("config_version IS NULL OR config_version <> 1").Count(&invalid).Error; err != nil || invalid != 0 {
		t.Fatalf("backfill invalid rows=%d error=%v", invalid, err)
	}
	if err := database.Exec("UPDATE p115_accounts SET config_version = 7").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(string(content)).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&models.P115Account{}).Where("config_version <> 7").Count(&invalid).Error; err != nil || invalid != 0 {
		t.Fatalf("repeat reset generations=%d error=%v", invalid, err)
	}
}
