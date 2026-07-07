package policy

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestInsertUserMediaLibraryPreferenceTxIncludesDisabledFlag(t *testing.T) {
	database, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  "host=127.0.0.1 user=test dbname=test sslmode=disable",
		PreferSimpleProtocol: true,
	}), &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true,
	})
	if err != nil {
		t.Fatalf("open dry-run database: %v", err)
	}

	preference := models.UserMediaLibraryPreference{
		UserID:    "user_1",
		LibraryID: "lib_disabled",
		Enabled:   false,
	}
	sql := database.ToSQL(func(tx *gorm.DB) *gorm.DB {
		return insertUserMediaLibraryPreferenceTx(tx, &preference)
	})
	sql = strings.Join(strings.Fields(sql), " ")

	assertSQLContains(t, sql, `"enabled"`)
	assertSQLContains(t, sql, "false")
}

func TestEnabledSetMatchesTemplate(t *testing.T) {
	libraries := []models.PlanGroupMediaLibrary{
		{LibraryID: "lib_a"},
		{LibraryID: "lib_b"},
	}

	tests := []struct {
		name       string
		libraries  []models.PlanGroupMediaLibrary
		enabledSet map[string]struct{}
		want       bool
	}{
		{
			name:      "all template libraries enabled",
			libraries: libraries,
			enabledSet: map[string]struct{}{
				"lib_a": {},
				"lib_b": {},
			},
			want: true,
		},
		{
			name:      "missing template library",
			libraries: libraries,
			enabledSet: map[string]struct{}{
				"lib_a": {},
			},
			want: false,
		},
		{
			name:      "outside library does not make missing template library valid",
			libraries: libraries,
			enabledSet: map[string]struct{}{
				"lib_a": {},
				"lib_x": {},
			},
			want: false,
		},
		{
			name:       "empty template matches empty selection",
			libraries:  nil,
			enabledSet: map[string]struct{}{},
			want:       true,
		},
		{
			name:      "outside library is ignored once template is fully enabled",
			libraries: libraries,
			enabledSet: map[string]struct{}{
				"lib_a": {},
				"lib_b": {},
				"lib_x": {},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := enabledSetMatchesTemplate(tt.libraries, tt.enabledSet); got != tt.want {
				t.Fatalf("expected %t, got %t", tt.want, got)
			}
		})
	}
}

func TestToLibraryOptionsFiltersSystemCollections(t *testing.T) {
	options := toLibraryOptions([]embyint.EmbyLibrary{
		{ID: " lib_movies ", Name: " 电影 ", Type: " movies ", ItemCount: 12},
		{ID: "lib_collections", Name: "合集", Type: "BoxSets"},
		{ID: "lib_series", Name: "剧集", Type: "tvshows", ItemCount: 8},
		{ID: " ", Name: "空 ID", Type: "movies"},
	})

	if len(options) != 2 {
		t.Fatalf("expected 2 ordinary libraries, got %d: %+v", len(options), options)
	}
	for _, option := range options {
		if option.ID == "lib_collections" {
			t.Fatalf("expected system collections to be filtered, got %+v", options)
		}
	}
	optionByID := mediaLibraryOptionMap(options)
	movieOption := optionByID["lib_movies"]
	if movieOption.ID != "lib_movies" || movieOption.Name != "电影" || movieOption.Type != "movies" || movieOption.ItemCount != 12 {
		t.Fatalf("expected library fields to be trimmed and copied, got %+v", movieOption)
	}
}

func TestNormalizePolicyLibraryIDsTrimsDeduplicatesAndSorts(t *testing.T) {
	got, err := normalizePolicyLibraryIDs([]string{" lib_b ", "", "lib_a", "lib_b", " lib_c "})
	if err != nil {
		t.Fatalf("normalize policy library ids: %v", err)
	}

	want := []string{"lib_a", "lib_b", "lib_c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %+v, got %+v", want, got)
	}
}

func TestIsUserMediaLibraryTemplateOutOfSync(t *testing.T) {
	group := &models.PlanGroup{Key: "VIP", MediaLibraryTemplateVersion: 3}

	if !isUserMediaLibraryTemplateOutOfSync(&models.User{
		ID:                                 "user_1",
		Role:                               "user",
		EmbyID:                             "emby_1",
		AppliedMediaLibraryTemplateVersion: 2,
	}, group) {
		t.Fatal("expected managed user with stale template version to be out of sync")
	}

	if isUserMediaLibraryTemplateOutOfSync(&models.User{
		ID:                                 "user_1",
		Role:                               "user",
		EmbyID:                             "emby_1",
		AppliedMediaLibraryTemplateVersion: 3,
	}, group) {
		t.Fatal("expected equal template version to be synced")
	}

	if isUserMediaLibraryTemplateOutOfSync(&models.User{
		ID:                                 "admin_1",
		Role:                               "admin",
		EmbyID:                             "emby_admin",
		AppliedMediaLibraryTemplateVersion: 1,
	}, group) {
		t.Fatal("expected admin user to skip out-of-sync detection")
	}

	if isUserMediaLibraryTemplateOutOfSync(&models.User{
		ID:                                 "user_2",
		Role:                               "user",
		AppliedMediaLibraryTemplateVersion: 1,
	}, group) {
		t.Fatal("expected unbound Emby user to skip out-of-sync detection")
	}
}

func TestPolicyValueHelpersKeepOnlyExpectedTypes(t *testing.T) {
	if !boolPolicyValue(true) {
		t.Fatal("expected bool true to be accepted")
	}
	if boolPolicyValue("true") || boolPolicyValue(1) || boolPolicyValue(false) {
		t.Fatal("expected non-bool or false policy values to be false")
	}

	if got := stringSlicePolicyValue([]string{"lib_a", "lib_b"}); !reflect.DeepEqual(got, []string{"lib_a", "lib_b"}) {
		t.Fatalf("unexpected string slice policy value: %+v", got)
	}
	if got := stringSlicePolicyValue([]any{"lib_a", 1, "lib_b", nil}); !reflect.DeepEqual(got, []string{"lib_a", "lib_b"}) {
		t.Fatalf("unexpected any slice policy value: %+v", got)
	}
	if got := stringSlicePolicyValue("lib_a"); got != nil {
		t.Fatalf("expected unsupported policy value to be nil, got %+v", got)
	}
}

func TestMediaLibraryOptionHelpers(t *testing.T) {
	options := []MediaLibraryOption{
		{ID: "lib_a", Name: "电影"},
		{ID: "lib_b", Name: "剧集"},
	}

	optionByID := mediaLibraryOptionMap(options)
	if optionByID["lib_a"].Name != "电影" || optionByID["lib_b"].Name != "剧集" {
		t.Fatalf("unexpected option map: %+v", optionByID)
	}
	if got := mediaLibraryOptionIDs(options); !reflect.DeepEqual(got, []string{"lib_a", "lib_b"}) {
		t.Fatalf("unexpected option ids: %+v", got)
	}

	built := buildLibraryOptionsFromIDs([]string{"lib_b", "lib_missing", "lib_a"}, optionByID)
	if len(built) != 3 {
		t.Fatalf("expected 3 built options, got %d", len(built))
	}
	if built[0].ID != "lib_b" || built[0].Name != "剧集" {
		t.Fatalf("expected known option for lib_b, got %+v", built[0])
	}
	if built[1].ID != "lib_missing" || built[1].Name != "lib_missing" {
		t.Fatalf("expected fallback option name, got %+v", built[1])
	}
}

func TestFindAndNormalizeUserIDs(t *testing.T) {
	users := []models.User{
		{ID: "user_a", Username: "A"},
		{ID: "user_b", Username: "B"},
	}

	found := findUserInPreviewUsers(users, "user_b")
	if found == nil || found.Username != "B" {
		t.Fatalf("expected to find user_b, got %+v", found)
	}
	if missing := findUserInPreviewUsers(users, "user_x"); missing != nil {
		t.Fatalf("expected missing user to return nil, got %+v", missing)
	}

	got := normalizeUserIDs([]string{" user_a ", "", "user_b", "user_a", " user_c "})
	want := []string{"user_a", "user_b", "user_c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %+v, got %+v", want, got)
	}
}

func TestIDSetAndPlanGroupLibraryIDs(t *testing.T) {
	set := idSet([]string{"lib_a", "lib_b", "lib_a"})
	if _, ok := set["lib_a"]; !ok {
		t.Fatalf("expected lib_a in set: %+v", set)
	}
	if _, ok := set["lib_b"]; !ok {
		t.Fatalf("expected lib_b in set: %+v", set)
	}
	if _, ok := set["lib_c"]; ok {
		t.Fatalf("unexpected lib_c in set: %+v", set)
	}

	got := planGroupLibraryIDs([]models.PlanGroupMediaLibrary{
		{LibraryID: "lib_a"},
		{LibraryID: "lib_b"},
	})
	if !reflect.DeepEqual(got, []string{"lib_a", "lib_b"}) {
		t.Fatalf("unexpected library ids: %+v", got)
	}
}

func TestNormalizeEnabledSetRejectsLibrariesOutsideTemplate(t *testing.T) {
	libraries := []models.PlanGroupMediaLibrary{
		{LibraryID: "lib_a"},
		{LibraryID: "lib_b"},
	}

	enabledSet, err := normalizeEnabledSet([]string{" lib_a ", "", "lib_a", "lib_b"}, libraries)
	if err != nil {
		t.Fatalf("normalize enabled set: %v", err)
	}
	if _, ok := enabledSet["lib_a"]; !ok {
		t.Fatalf("expected lib_a enabled: %+v", enabledSet)
	}
	if _, ok := enabledSet["lib_b"]; !ok {
		t.Fatalf("expected lib_b enabled: %+v", enabledSet)
	}

	if _, err := normalizeEnabledSet([]string{"lib_x"}, libraries); !errors.Is(err, ErrLibraryOutsideTemplate) {
		t.Fatalf("expected ErrLibraryOutsideTemplate, got %v", err)
	}
}

func TestEnabledSetFromPreferencesDefaultsToTemplateThenHonorsExplicitPreferences(t *testing.T) {
	libraries := []models.PlanGroupMediaLibrary{
		{LibraryID: "lib_a"},
		{LibraryID: "lib_b"},
	}

	defaultEnabled := enabledSetFromPreferences(libraries, nil)
	if _, ok := defaultEnabled["lib_a"]; !ok {
		t.Fatalf("expected template library lib_a enabled by default: %+v", defaultEnabled)
	}
	if _, ok := defaultEnabled["lib_b"]; !ok {
		t.Fatalf("expected template library lib_b enabled by default: %+v", defaultEnabled)
	}

	explicitEnabled := enabledSetFromPreferences(libraries, []models.UserMediaLibraryPreference{
		{LibraryID: "lib_a", Enabled: true},
		{LibraryID: "lib_b", Enabled: false},
		{LibraryID: "lib_x", Enabled: true},
	})
	if _, ok := explicitEnabled["lib_a"]; !ok {
		t.Fatalf("expected lib_a enabled from preference: %+v", explicitEnabled)
	}
	if _, ok := explicitEnabled["lib_b"]; ok {
		t.Fatalf("expected disabled lib_b to be absent: %+v", explicitEnabled)
	}
	if _, ok := explicitEnabled["lib_x"]; !ok {
		t.Fatalf("expected current behavior to preserve enabled outside-template preference: %+v", explicitEnabled)
	}
}

func TestBuildPolicyTemplateResponseCopiesTemplateAndGroupFields(t *testing.T) {
	response := buildPolicyTemplateResponse(
		&models.PlanGroup{Key: "VIP", Name: "高级用户"},
		models.PlanGroupEmbyPolicyTemplate{
			SimultaneousStreamLimit:        3,
			EnableContentDownloading:       true,
			EnableLiveTvAccess:             true,
			EnableSyncTranscoding:          false,
			EnableAudioPlaybackTranscoding: true,
			EnableVideoPlaybackTranscoding: false,
			EnablePlaybackRemuxing:         true,
			EnableRemoteAccess:             false,
		},
		12,
	)

	if response.PlanGroupKey != "VIP" || response.PlanGroupName != "高级用户" || response.AffectedUserCount != 12 {
		t.Fatalf("unexpected response basics: %+v", response)
	}
	if response.SimultaneousStreamLimit != 3 || !response.EnableContentDownloading || !response.EnableLiveTvAccess {
		t.Fatalf("unexpected policy response flags: %+v", response)
	}
	if response.EnableSyncTranscoding || response.EnableVideoPlaybackTranscoding || response.EnableRemoteAccess {
		t.Fatalf("unexpected disabled policy response flags: %+v", response)
	}
}

func TestSystemCollectionPtrTimeAndTruncateErrorHelpers(t *testing.T) {
	if !isSystemCollectionLibraryType(" BoxSets ") {
		t.Fatal("expected boxsets to be treated as a system collection library type")
	}
	if isSystemCollectionLibraryType("movies") {
		t.Fatal("expected movies to be treated as ordinary library type")
	}

	now := time.Date(2026, 4, 19, 10, 0, 0, 0, time.UTC)
	if got := ptrTime(now); got == nil || !got.Equal(now) {
		t.Fatalf("unexpected time pointer: %+v", got)
	}
	if got := truncateError(nil); got != "" {
		t.Fatalf("expected nil error to truncate to blank string, got %q", got)
	}
	shortErr := errors.New("短错误")
	if got := truncateError(shortErr); got != "短错误" {
		t.Fatalf("unexpected short error: %q", got)
	}
	longErr := errors.New(strings.Repeat("x", 520))
	if got := truncateError(longErr); len(got) != 500 {
		t.Fatalf("expected long error to be truncated to 500 chars, got %d", len(got))
	}
}
