package policy

import (
	"testing"

	"github.com/konghang/ember/backend/internal/models"
)

func TestBuildManagedPolicyFieldsPreservesOnlyManagedBoundary(t *testing.T) {
	template := models.PlanGroupEmbyPolicyTemplate{
		SimultaneousStreamLimit:        5,
		EnableContentDownloading:       true,
		EnableLiveTvAccess:             false,
		EnableSyncTranscoding:          true,
		EnableAudioPlaybackTranscoding: false,
		EnableVideoPlaybackTranscoding: true,
		EnablePlaybackRemuxing:         true,
		EnableRemoteAccess:             false,
	}

	managed, fields := buildManagedPolicyFields(map[string]any{
		"SimultaneousStreamLimit": 3,
		"ExcludedSubFolders":      []string{"keep"},
	}, true, template, []string{"lib_a", "lib_b"})

	assertFieldPresent(t, fields, "SimultaneousStreamLimit")
	assertFieldAbsent(t, fields, "ExcludedSubFolders")

	if managed["IsDisabled"] != true {
		t.Fatalf("expected disabled policy")
	}
	if managed["IsAdministrator"] != false || managed["EnableContentDeletion"] != false {
		t.Fatalf("expected forced safety fields, got %+v", managed)
	}
	if managed["EnableAllFolders"] != false {
		t.Fatalf("expected EnableAllFolders=false")
	}
	if got := managed["EnabledFolders"].([]string); len(got) != 2 || got[0] != "lib_a" || got[1] != "lib_b" {
		t.Fatalf("unexpected enabled folders: %+v", got)
	}
	if managed["SimultaneousStreamLimit"] != 5 {
		t.Fatalf("expected simultaneous stream limit from template, got %+v", managed["SimultaneousStreamLimit"])
	}
}

func TestBuildManagedPolicyFieldsUsesMaxActiveSessionsCompatibility(t *testing.T) {
	template := models.PlanGroupEmbyPolicyTemplate{SimultaneousStreamLimit: 2}

	managed, fields := buildManagedPolicyFields(map[string]any{
		"MaxActiveSessions": 1,
	}, false, template, nil)

	assertFieldPresent(t, fields, "MaxActiveSessions")
	assertFieldAbsent(t, fields, "SimultaneousStreamLimit")
	if managed["MaxActiveSessions"] != 2 {
		t.Fatalf("expected MaxActiveSessions from template, got %+v", managed["MaxActiveSessions"])
	}
}

func TestShouldApplyEffectivePolicyToUserSkipsAdmins(t *testing.T) {
	if shouldApplyEffectivePolicyToUser(&models.User{ID: "admin_1", Role: "admin", EmbyID: "emby_admin"}) {
		t.Fatalf("expected admin user to skip managed policy sync")
	}
	if shouldApplyEffectivePolicyToUser(&models.User{ID: "user_1", Role: "user"}) {
		t.Fatalf("expected unbound user to skip managed policy sync")
	}
	if !shouldApplyEffectivePolicyToUser(&models.User{ID: "user_2", Role: "user", EmbyID: "emby_user"}) {
		t.Fatalf("expected ordinary bound user to receive managed policy sync")
	}
}

func TestIsEmbyAdministratorPolicy(t *testing.T) {
	if !isEmbyAdministratorPolicy(map[string]any{"IsAdministrator": true}) {
		t.Fatalf("expected Emby administrator policy to be detected")
	}
	if isEmbyAdministratorPolicy(map[string]any{"IsAdministrator": false}) {
		t.Fatalf("expected ordinary Emby policy to stay eligible")
	}
	if isEmbyAdministratorPolicy(map[string]any{}) {
		t.Fatalf("expected missing administrator flag to stay eligible")
	}
}

func assertFieldPresent(t *testing.T, fields []string, want string) {
	t.Helper()
	for _, field := range fields {
		if field == want {
			return
		}
	}
	t.Fatalf("expected field %s in %+v", want, fields)
}

func assertFieldAbsent(t *testing.T, fields []string, want string) {
	t.Helper()
	for _, field := range fields {
		if field == want {
			t.Fatalf("expected field %s to be absent from %+v", want, fields)
		}
	}
}
