package app

import (
	"bytes"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/konghang/ember/backend/internal/common"
	dbpkg "github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const integrationDatabaseURLEnv = "EMBER_INTEGRATION_DATABASE_URL"

// integrationHarness 提供 API 进程内集成测试的最小基座：
// - 独立 schema 隔离
// - 启动期 migration / VerifySchema / Bootstrap
// - 真实 router + JWT 鉴权
//
// 约束：只用于仓内集成测试，不触发真实外部链路；未来需要 Emby / MoviePilot /
// Telegram 时，在用例内通过 settings 指向 httptest fake server。
type integrationHarness struct {
	router     *gin.Engine
	database   *gorm.DB
	adminToken string
	adminUser  models.User
	schemaName string
}

func newIntegrationHarness(t *testing.T) *integrationHarness {
	t.Helper()

	baseDSN := strings.TrimSpace(os.Getenv(integrationDatabaseURLEnv))
	if baseDSN == "" {
		t.Skipf("未设置 %s，跳过 API 集成测试骨架；请提供指向隔离 PostgreSQL 的连接串", integrationDatabaseURLEnv)
	}

	t.Setenv("EMBER_MIGRATIONS_DIR", integrationMigrationsDir(t))
	t.Setenv("JWT_SECRET", strings.Repeat("j", 32))
	if err := common.InitJWT(); err != nil {
		t.Fatalf("InitJWT(): %v", err)
	}

	adminDB := openIntegrationDatabase(t, baseDSN)
	schemaName := integrationSchemaName(t.Name())
	if err := adminDB.Exec(fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS "%s"`, schemaName)).Error; err != nil {
		t.Fatalf("create schema %s: %v", schemaName, err)
	}

	appDB := openIntegrationDatabase(t, baseDSN)
	appSQLDB, err := appDB.DB()
	if err != nil {
		t.Fatalf("appDB.DB(): %v", err)
	}
	appSQLDB.SetMaxOpenConns(1)
	appSQLDB.SetMaxIdleConns(1)
	if err := appDB.Exec(fmt.Sprintf(`SET search_path TO "%s", public`, schemaName)).Error; err != nil {
		t.Fatalf("set search_path: %v", err)
	}

	previousDB := dbpkg.DB
	dbpkg.DB = appDB
	t.Cleanup(func() {
		dbpkg.DB = previousDB

		if sqlDB, err := appDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
		if err := adminDB.Exec(fmt.Sprintf(`DROP SCHEMA IF EXISTS "%s" CASCADE`, schemaName)).Error; err != nil {
			t.Fatalf("drop schema %s: %v", schemaName, err)
		}
		if sqlDB, err := adminDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	if err := dbpkg.Migrate(); err != nil {
		t.Fatalf("Migrate(): %v", err)
	}
	if err := dbpkg.VerifySchema(); err != nil {
		t.Fatalf("VerifySchema(): %v", err)
	}
	dbpkg.Bootstrap()

	adminUser := seedIntegrationAdminUser(t, appDB)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.Recovery())
	registerRoutes(router, newAppHandlers())

	return &integrationHarness{
		router:     router,
		database:   appDB,
		adminToken: integrationAdminToken(t, adminUser),
		adminUser:  adminUser,
		schemaName: schemaName,
	}
}

func (h *integrationHarness) performAdminRequest(method, target string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+h.adminToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	h.router.ServeHTTP(recorder, req)
	return recorder
}

func (h *integrationHarness) setSetting(t *testing.T, key, value string) {
	t.Helper()
	record := models.Setting{
		Key:   strings.TrimSpace(key),
		Value: value,
	}
	if err := h.database.Save(&record).Error; err != nil {
		t.Fatalf("set setting %s: %v", key, err)
	}
}

func openIntegrationDatabase(t *testing.T, dsn string) *gorm.DB {
	t.Helper()

	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		DisableAutomaticPing: true,
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	return database
}

func seedIntegrationAdminUser(t *testing.T, database *gorm.DB) models.User {
	t.Helper()

	admin := models.User{
		Username: "itest_admin",
		Role:     "admin",
		Email:    "itest-admin@example.com",
		IsActive: true,
	}
	if err := admin.SetPassword("integration-secret"); err != nil {
		t.Fatalf("SetPassword(): %v", err)
	}
	if err := database.Create(&admin).Error; err != nil {
		t.Fatalf("create admin user: %v", err)
	}
	return admin
}

func integrationAdminToken(t *testing.T, admin models.User) string {
	t.Helper()

	token, err := common.GenerateToken(
		admin.ID,
		admin.Username,
		admin.Role,
		common.ComputePasswordSignature(admin.Password),
		3600,
	)
	if err != nil {
		t.Fatalf("GenerateToken(): %v", err)
	}
	return token
}

func integrationMigrationsDir(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	dir := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "infrastructure", "database")
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}
	return abs
}

func integrationSchemaName(testName string) string {
	sanitized := strings.ToLower(testName)
	replacer := strings.NewReplacer("/", "_", " ", "_", "-", "_", ".", "_")
	sanitized = replacer.Replace(sanitized)
	return fmt.Sprintf("itest_%s_%d", sanitized, time.Now().UnixNano())
}
