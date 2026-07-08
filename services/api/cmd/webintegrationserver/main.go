package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	apppkg "github.com/konghang/ember/backend/internal/app"
	"github.com/konghang/ember/backend/internal/common"
	dbpkg "github.com/konghang/ember/backend/internal/db"
	logpkg "github.com/konghang/ember/backend/internal/logging"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

const (
	testAdminUsername = "itest_admin_web"
	testAdminPassword = "integration-admin-secret"
	testUserUsername  = "itest_user_web"
	testUserPassword  = "integration-user-secret"
	testVIPGroupKey   = "VIP"
)

func main() {
	if err := logpkg.Init(); err != nil {
		log.Fatalf("init logging: %v", err)
	}

	if err := ensureEnv("DATABASE_URL"); err != nil {
		log.Fatal(err)
	}
	if strings.TrimSpace(os.Getenv("JWT_SECRET")) == "" {
		_ = os.Setenv("JWT_SECRET", strings.Repeat("j", 32))
	}
	if strings.TrimSpace(os.Getenv("INTERNAL_API_SECRET")) == "" {
		_ = os.Setenv("INTERNAL_API_SECRET", "0123456789abcdef0123456789abcdef")
	}

	fakeEmby := newFakeEmbyServer()
	defer fakeEmby.Close()

	_ = os.Setenv("EMBY_URL", fakeEmby.URL)
	_ = os.Setenv("EMBY_API_KEY", "integration-emby-key")
	_ = os.Unsetenv("BOT_NOTIFY_URL")
	_ = os.Unsetenv("MOVIEPILOT_URL")
	_ = os.Unsetenv("MOVIEPILOT_API_KEY")

	dbpkg.InitDB()
	defer dbpkg.Close()

	if err := resetPublicSchema(dbpkg.DB); err != nil {
		log.Fatalf("reset public schema: %v", err)
	}
	if err := dbpkg.Migrate(); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if err := dbpkg.VerifySchema(); err != nil {
		log.Fatalf("verify schema: %v", err)
	}
	dbpkg.Bootstrap()

	if err := seedWebIntegrationData(dbpkg.DB); err != nil {
		log.Fatalf("seed web integration data: %v", err)
	}

	if err := common.InitJWT(); err != nil {
		log.Fatalf("init jwt: %v", err)
	}
	if err := common.InitInternalAPISecret(); err != nil {
		log.Fatalf("init internal secret: %v", err)
	}

	if err := apppkg.Start(); err != nil {
		log.Fatalf("start app: %v", err)
	}
}

func ensureEnv(key string) error {
	if strings.TrimSpace(os.Getenv(key)) == "" {
		return fmt.Errorf("%s is required", key)
	}
	return nil
}

func resetPublicSchema(database *gorm.DB) error {
	if database == nil {
		return fmt.Errorf("database is nil")
	}
	statements := []string{
		`DROP SCHEMA IF EXISTS public CASCADE`,
		`CREATE SCHEMA public`,
		`GRANT ALL ON SCHEMA public TO public`,
	}
	for _, statement := range statements {
		if err := database.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func seedWebIntegrationData(database *gorm.DB) error {
	if err := seedIntegrationSettings(database); err != nil {
		return err
	}
	if err := seedAdminUser(database); err != nil {
		return err
	}
	if err := seedVIPPlanGroup(database); err != nil {
		return err
	}
	return seedBoundUser(database)
}

func seedAdminUser(database *gorm.DB) error {
	admin := models.User{
		Username: testAdminUsername,
		Role:     "admin",
		Email:    "itest-admin-web@example.com",
		IsActive: true,
	}
	if err := admin.SetPassword(testAdminPassword); err != nil {
		return err
	}
	return database.Create(&admin).Error
}

func seedVIPPlanGroup(database *gorm.DB) error {
	group := models.PlanGroup{
		Key:                         testVIPGroupKey,
		Name:                        "VIP",
		SortOrder:                   10,
		MediaLibraryTemplateVersion: 1,
	}
	if err := database.Create(&group).Error; err != nil {
		return err
	}
	template := models.PlanGroupEmbyPolicyTemplate{
		PlanGroupKey:            testVIPGroupKey,
		SimultaneousStreamLimit: 3,
		EnablePlaybackRemuxing:  true,
		EnableRemoteAccess:      true,
	}
	return database.Create(&template).Error
}

func seedBoundUser(database *gorm.DB) error {
	planGroup := testVIPGroupKey
	user := models.User{
		Username:                           testUserUsername,
		Role:                               "user",
		Email:                              "itest-user-web@example.com",
		EmbyID:                             "emby_user_policy",
		PlanGroup:                          &planGroup,
		AppliedMediaLibraryTemplateVersion: 1,
		IsActive:                           true,
	}
	if err := user.SetPassword(testUserPassword); err != nil {
		return err
	}
	return database.Create(&user).Error
}

func seedIntegrationSettings(database *gorm.DB) error {
	settings := []models.Setting{
		{Key: "EMBY_URL", Value: os.Getenv("EMBY_URL")},
		{Key: "EMBY_API_KEY", Value: os.Getenv("EMBY_API_KEY")},
	}
	for _, setting := range settings {
		if err := database.Where("key = ?", setting.Key).Delete(&models.Setting{}).Error; err != nil {
			return err
		}
		if err := database.Create(&setting).Error; err != nil {
			return err
		}
	}
	return nil
}

func newFakeEmbyServer() *httptest.Server {
	userPolicy := map[string]any{
		"IsAdministrator":                false,
		"IsDisabled":                     false,
		"EnableContentDeletion":          false,
		"EnableContentDownloading":       false,
		"EnableAllFolders":               false,
		"EnabledFolders":                 []any{},
		"EnableLiveTvAccess":             false,
		"EnableSyncTranscoding":          false,
		"EnableMediaPlayback":            true,
		"EnableAudioPlaybackTranscoding": false,
		"EnableVideoPlaybackTranscoding": false,
		"EnablePlaybackRemuxing":         true,
		"EnableRemoteAccess":             true,
		"SimultaneousStreamLimit":        3,
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/emby/Users/AuthenticateByName":
			var payload struct {
				Username string `json:"Username"`
				Pw       string `json:"Pw"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if payload.Username != testUserUsername || payload.Pw != testUserPassword {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"User": map[string]any{
					"Id":   "emby_user_policy",
					"Name": testUserUsername,
				},
			})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/emby/Library/VirtualFolders/Query":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `[
				{"Id":"/data/movies","Name":"电影","CollectionType":"movies","ItemCount":12},
				{"Id":"/data/series","Name":"剧集","CollectionType":"tvshows","ItemCount":8}
			]`)
			return
		case r.Method == http.MethodGet && r.URL.Path == "/emby/Users":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"Id":   "emby_admin",
					"Name": "integration-admin",
					"Policy": map[string]any{
						"IsAdministrator": true,
					},
				},
				{
					"Id":   "emby_user_policy",
					"Name": testUserUsername,
					"Policy": map[string]any{
						"IsAdministrator": false,
					},
				},
			})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/emby/Users/emby_admin/Views":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Items": []map[string]any{
					{"Id": "/data/movies", "Name": "电影", "CollectionType": "movies"},
					{"Id": "/data/series", "Name": "剧集", "CollectionType": "tvshows"},
				},
			})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/emby/Users/emby_user_policy":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Id":     "emby_user_policy",
				"Name":   testUserUsername,
				"Policy": userPolicy,
			})
			return
		case r.Method == http.MethodPost && r.URL.Path == "/emby/Users/emby_user_policy/Policy":
			defer r.Body.Close()
			var next map[string]any
			if err := json.NewDecoder(r.Body).Decode(&next); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			userPolicy = cloneJSONMap(next)
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
}

func cloneJSONMap(input map[string]any) map[string]any {
	body, _ := json.Marshal(input)
	var cloned map[string]any
	_ = json.NewDecoder(bytes.NewReader(body)).Decode(&cloned)
	return cloned
}
