package policy

import (
	"strings"
	"testing"

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
		{ID: "lib_movies", Name: "电影", Type: "movies"},
		{ID: "lib_collections", Name: "合集", Type: "BoxSets"},
		{ID: "lib_series", Name: "剧集", Type: "tvshows"},
	})

	if len(options) != 2 {
		t.Fatalf("expected 2 ordinary libraries, got %d: %+v", len(options), options)
	}
	for _, option := range options {
		if option.ID == "lib_collections" {
			t.Fatalf("expected system collections to be filtered, got %+v", options)
		}
	}
}
