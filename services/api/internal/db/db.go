package db

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	logpkg "github.com/konghang/ember/backend/internal/logging"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// InitDB 初始化数据库连接
func InitDB() {
	if envPath := os.Getenv("EMBER_DOTENV"); envPath != "" {
		if err := godotenv.Load(envPath); err != nil {
			log.Printf("[Env] godotenv.Load(%s) 失败：%v", envPath, err)
		} else {
			log.Printf("✅ 成功加载环境变量：%s", envPath)
		}
	} else if envPath := defaultDotenvPath(); envPath != "" {
		if err := godotenv.Load(envPath); err != nil {
			log.Printf("[Env] godotenv.Load(%s) 失败：%v", envPath, err)
		} else {
			log.Printf("✅ 成功加载环境变量：%s", envPath)
		}
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("❌ DATABASE_URL 环境变量未设置；本地启动请确认 .env 或 services/api/.env 存在，或先 export DATABASE_URL")
	}
	dsn = withConnectTimeout(dsn, "8")

	// 创建自定义 logger，显示详细的 SQL 日志
	newLogger := logger.New(
		log.New(logpkg.Writer(), "\r\n", log.LstdFlags), // io writer
		logger.Config{
			SlowThreshold:             time.Second, // 慢查询阈值
			LogLevel:                  logger.Info, // 日志级别：Info 显示所有 SQL
			IgnoreRecordNotFoundError: false,       // 不忽略 RecordNotFound 错误
			Colorful:                  true,        // 彩色输出
		},
	)

	var err error
	log.Println("ℹ️  正在连接 PostgreSQL...")
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: newLogger,

		NowFunc: func() time.Time {
			return time.Now().UTC()
		},

		// 开启预编译语句缓存
		PrepareStmt: true,

		// 查询时包含字段名
		QueryFields: true,
	})
	if err != nil {
		log.Fatalf("❌ 无法连接数据库：%v", err)
	}

	sqlDB, err := DB.DB()
	if err != nil {
		log.Fatalf("❌ 无法获取 SQL DB：%v", err)
	}

	// 连接池配置
	sqlDB.SetMaxIdleConns(15)
	sqlDB.SetMaxOpenConns(30)
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(10 * time.Minute)

	// 测试连接
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer pingCancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		log.Fatalf("❌ 数据库无法 ping 通：%v", err)
	}

	// 获取 PostgreSQL 版本
	var pgVersion string
	DB.Raw("SHOW server_version").Scan(&pgVersion)
	fmt.Printf("✅ PostgreSQL 版本：%s\n", pgVersion)

	fmt.Println("✅ 数据库连接成功")
}

func defaultDotenvPath() string {
	for _, path := range []string{".env", "services/api/.env"} {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// VerifySchema 在启动期检查所有业务表、关键列与关键索引是否存在。
//
// schema 由 infrastructure/database/ 下的 SQL migration 维护，启动期不再调用
// AutoMigrate；如果运维漏跑某条 SQL，本函数会在第一时间 fail-fast，避免 API
// 跑到第一次查询时才报错。
//
// 校验分三层：
//  1. 表存在：覆盖 baseline 与"新建表"型增量 migration
//  2. 关键列：覆盖"在已有表上加列"型增量 migration（仅检查每条 migration 引入的有代表性列）
//  3. 关键索引：覆盖"加约束 / 加 partial unique"型增量 migration
//
// 列与索引指纹按 migration 文件维护；新增顶层 SQL 时必须在 schemaFingerprints
// 中追加对应条目，否则该 migration 漏跑不会被本函数发现。
func VerifySchema() error {
	tableChecks := []struct {
		name  string
		model interface{}
	}{
		{"users", &models.User{}},
		{"redemption_codes", &models.RedemptionCode{}},
		{"redemptions", &models.Redemption{}},
		{"settings", &models.Setting{}},
		{"plan_groups", &models.PlanGroup{}},
		{"plans", &models.Plan{}},
		{"payments", &models.Payment{}},
		{"subscriptions", &models.Subscription{}},
		{"media_gaps", &models.MediaGap{}},
		{"playback_rankings", &models.PlaybackRanking{}},
		{"media_quality_caches", &models.MediaQualityCache{}},
		{"client_blacklists", &models.ClientBlacklist{}},
		{"device_actions", &models.DeviceAction{}},
		{"email_verifications", &models.EmailVerification{}},
		{"telegram_bind_codes", &models.TelegramBindCode{}},
		{"tv_calendar_sources", &models.TVCalendarSource{}},
		{"tv_calendar_items", &models.TVCalendarItem{}},
		{"tv_calendar_subscriptions", &models.TVCalendarSubscription{}},
		{"tmdb_cache", &models.TMDBCache{}},
		{"failed_emby_async_ops", &models.FailedEmbyAsyncOp{}},
		{"stripe_webhook_events", &models.StripeWebhookEvent{}},
		{"media_gap_scans", &models.MediaGapScan{}},
		{"bot_pending_reject_requests", &models.BotPendingRejectRequest{}},
		{"bot_runtime_locks", &models.BotRuntimeLock{}},
		{"subscription_admin_notifications", &models.SubscriptionAdminNotification{}},
		{"plan_group_media_libraries", &models.PlanGroupMediaLibrary{}},
		{"plan_group_emby_policy_templates", &models.PlanGroupEmbyPolicyTemplate{}},
		{"user_media_library_preferences", &models.UserMediaLibraryPreference{}},
		{"emby_policy_sync_batches", &models.EmbyPolicySyncBatch{}},
		{"emby_policy_sync_tasks", &models.EmbyPolicySyncTask{}},
	}

	modelByTable := make(map[string]interface{}, len(tableChecks))
	var missingTables []string
	for _, c := range tableChecks {
		modelByTable[c.name] = c.model
		if !DB.Migrator().HasTable(c.model) {
			missingTables = append(missingTables, c.name)
		}
	}
	if len(missingTables) > 0 {
		return fmt.Errorf("数据库缺少必要的表：%s；请按 infrastructure/database/README.md 执行 SQL migration（本地空库可直接 `go run ./cmd/server`，启动期会自动应用）", strings.Join(missingTables, ", "))
	}

	var missingColumns []string
	for _, c := range schemaFingerprintColumns {
		model, ok := modelByTable[c.table]
		if !ok {
			return fmt.Errorf("schemaFingerprintColumns 引用未注册的表 %q", c.table)
		}
		if !DB.Migrator().HasColumn(model, c.column) {
			missingColumns = append(missingColumns, fmt.Sprintf("%s.%s（来自 %s）", c.table, c.column, c.migration))
		}
	}
	if len(missingColumns) > 0 {
		return fmt.Errorf("数据库缺少必要的列：%s；请按 infrastructure/database/README.md 执行 SQL migration", strings.Join(missingColumns, ", "))
	}

	var missingIndexes []string
	for _, c := range schemaFingerprintIndexes {
		model, ok := modelByTable[c.table]
		if !ok {
			return fmt.Errorf("schemaFingerprintIndexes 引用未注册的表 %q", c.table)
		}
		if !DB.Migrator().HasIndex(model, c.index) {
			missingIndexes = append(missingIndexes, fmt.Sprintf("%s.%s（来自 %s）", c.table, c.index, c.migration))
		}
	}
	if len(missingIndexes) > 0 {
		return fmt.Errorf("数据库缺少必要的索引：%s；请按 infrastructure/database/README.md 执行 SQL migration", strings.Join(missingIndexes, ", "))
	}

	log.Println("✅ 数据库 schema 校验通过")
	return nil
}

// schemaFingerprintColumn 描述某条 SQL migration 引入的代表性列，
// 用于在启动期 VerifySchema 中 fail-fast 漏跑情况。
type schemaFingerprintColumn struct {
	table     string
	column    string
	migration string
}

// schemaFingerprintIndex 描述某条 SQL migration 引入的代表性索引，
// 同样用于启动期 fail-fast。
type schemaFingerprintIndex struct {
	table     string
	index     string
	migration string
}

// schemaFingerprintColumns 列出当前 build 时间所有顶层增量 migration 引入的代表性列。
//
// 维护规则：每次在 infrastructure/database/ 新增顶层 migration 后，把其中"在已有表上 ADD COLUMN"
// 的代表性列追加到这里；漏维护会导致 VerifySchema 放过缺该 migration 的环境。
var schemaFingerprintColumns = []schemaFingerprintColumn{
	{"subscriptions", "reject_reason", "20260416_01_subscription_status_and_review_fields"},
	{"subscriptions", "reviewed_at", "20260416_01_subscription_status_and_review_fields"},
	{"subscriptions", "ingested_at", "20260416_01_subscription_status_and_review_fields"},
	{"subscriptions", "retry_from_id", "20260424_01_subscription_resubmission_after_rejection"},
	{"subscriptions", "ingest_progress", "20260426_05_subscriptions_ingest_progress"},
	{"media_gaps", "last_dispatch_error", "20260426_06_media_gaps_dispatch_failed"},
	{"media_gaps", "ignore_reason_code", "20260427_02_media_gaps_ignore_reason_code"},
	{"media_quality_caches", "inflight_until", "20260426_09_media_quality_caches_inflight"},
	{"tv_calendar_sources", "last_full_sync_at", "20260426_11_tv_calendar_sources_sync_markers"},
	{"bot_pending_reject_requests", "message_id", "20260427_04_bot_pending_reject_message_context"},
	{"users", "password_reset_required", "20260426_15_users_password_reset_required"},
	{"bot_runtime_locks", "expires_at", "20260427_01_bot_runtime_locks"},
	{"subscription_admin_notifications", "delivery_status", "20260519_01_subscription_admin_notifications"},
	{"users", "emby_access_disabled", "20260527_01_user_media_library_policy"},
	{"plan_groups", "subscription_auto_approve_daily_limit", "20260707_01_subscription_plan_group_auto_approval"},
	{"plan_group_media_libraries", "library_id", "20260527_01_user_media_library_policy"},
	{"plan_group_emby_policy_templates", "simultaneous_stream_limit", "20260527_01_user_media_library_policy"},
	{"subscriptions", "review_source", "20260707_01_subscription_plan_group_auto_approval"},
	{"user_media_library_preferences", "enabled", "20260527_01_user_media_library_policy"},
	{"emby_policy_sync_batches", "failed_count", "20260527_01_user_media_library_policy"},
	{"emby_policy_sync_tasks", "next_retry_at", "20260527_01_user_media_library_policy"},
}

// schemaFingerprintIndexes 列出当前 build 时间所有顶层增量 migration 引入的代表性索引。
//
// 维护规则：每次在 infrastructure/database/ 新增顶层 migration 后，把其中"CREATE [UNIQUE] INDEX"
// 的代表性索引追加到这里；尤其是带业务约束（partial unique / 复合唯一）的索引必须列入。
var schemaFingerprintIndexes = []schemaFingerprintIndex{
	{"media_gaps", "uk_media_gap_episode", "20260418_01_media_gaps"},
	{"subscriptions", "uq_subscriptions_active_media", "20260424_01_subscription_resubmission_after_rejection"},
	{"subscriptions", "idx_subscriptions_retry_from_id", "20260424_01_subscription_resubmission_after_rejection"},
	// baseline 内置 + 由 20260425_01 在已有库补齐：新装环境从 baseline 已建，
	// 老线上库通过 20260425_01_baseline_normalization_indexes 补齐
	{"playback_rankings", "idx_ranking_lookup", "20260425_01_baseline_normalization_indexes"},
	{"redemptions", "uq_redemptions_user_code", "20260425_01_baseline_normalization_indexes"},
	{"telegram_bind_codes", "uq_telegram_bind_codes_user", "20260425_02_telegram_bind_codes_user_unique"},
	{"users", "uq_users_username_lower", "20260426_01_users_lower_unique_indexes"},
	{"users", "uq_users_email_lower", "20260426_01_users_lower_unique_indexes"},
	{"failed_emby_async_ops", "idx_failed_emby_async_ops_next", "20260426_02_failed_emby_async_ops"},
	{"stripe_webhook_events", "stripe_webhook_events_pkey", "20260426_03_stripe_webhook_events"},
	{"payments", "uq_payments_pending_user_plan", "20260426_04_payments_checkout_constraints"},
	{"payments", "idx_payments_stripe_session_id", "20260426_04_payments_checkout_constraints"},
	{"media_gap_scans", "idx_media_gap_scans_started", "20260426_07_media_gap_scans"},
	{"users", "uq_users_telegram_id", "20260426_13_schema_alignment"},
	{"playback_rankings", "uq_playback_rankings_period", "20260426_08_playback_rankings_idempotency"},
	{"device_actions", "idx_device_actions_operator", "20260426_10_device_actions_operator_id"},
	{"media_quality_caches", "idx_media_quality_caches_inflight", "20260426_09_media_quality_caches_inflight"},
	{"bot_pending_reject_requests", "idx_bot_pending_reject_requests_chat", "20260426_12_bot_pending_reject_requests"},
	{"bot_runtime_locks", "idx_bot_runtime_locks_expires", "20260427_01_bot_runtime_locks"},
	{"users", "uniq_users_emby_id", "20260504_00_users-emby-id-unique"},
	{"subscription_admin_notifications", "idx_subscription_admin_notifications_subscription", "20260519_01_subscription_admin_notifications"},
	{"subscription_admin_notifications", "uq_subscription_admin_notifications_message", "20260519_01_subscription_admin_notifications"},
	{"subscriptions", "idx_subscriptions_auto_quota_user_reviewed", "20260707_01_subscription_plan_group_auto_approval"},
	{"plan_group_media_libraries", "uq_plan_group_media_libraries_group_library", "20260527_01_user_media_library_policy"},
	{"user_media_library_preferences", "uq_user_media_library_preferences_user_library", "20260527_01_user_media_library_policy"},
	{"emby_policy_sync_tasks", "uq_emby_policy_sync_tasks_user_active", "20260527_01_user_media_library_policy"},
}

// Bootstrap 写入默认管理员、默认 settings、默认 plan_groups 等启动期数据。
//
// 启动序列为 `InitDB → Migrate → VerifySchema → Bootstrap → Start`：先把 schema
// 应用齐再 seed，否则 seed 会因表不存在而失败；这一拆分同时让本地空库一步到位
// 由 `go run ./cmd/server` 自然接管。
func Bootstrap() {
	seedDefaultAdmin()
	seedDefaultSettings()
	seedDefaultPlanGroups()
	seedDefaultPlanGroupEmbyPolicyTemplates()
}

// withConnectTimeout 为 URL 形式的 PostgreSQL DSN 注入 connect_timeout（秒）
// 仅在未显式配置 connect_timeout 时追加，避免无响应网络导致长时间阻塞。
func withConnectTimeout(dsn string, seconds string) string {
	if strings.Contains(dsn, "connect_timeout=") {
		return dsn
	}
	if !strings.HasPrefix(dsn, "postgres://") && !strings.HasPrefix(dsn, "postgresql://") {
		return dsn
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}
	query := u.Query()
	query.Set("connect_timeout", seconds)
	u.RawQuery = query.Encode()
	return u.String()
}

// seedDefaultAdmin 初始化默认管理员账号
func seedDefaultAdmin() {
	var count int64
	DB.Model(&models.User{}).Where("role = ?", "admin").Count(&count)
	if count > 0 {
		return
	}

	username := os.Getenv("ADMIN_USERNAME")
	if username == "" {
		log.Println("⚠️  跳过 admin 初始化：ADMIN_USERNAME 未设置")
		return
	}

	password := os.Getenv("ADMIN_PASSWORD")
	password_reset_required := false
	if password == "" {
		// 生成随机临时口令，要求首次登录立即修改
		b := make([]byte, 12)
		_, _ = rand.Read(b)
		password = base64.StdEncoding.EncodeToString(b)
		password_reset_required = true
		log.Printf("⚠️  [Admin Seed] ADMIN_PASSWORD 未设置，已生成临时口令，请立即登录并修改密码：%s", password)
	}

	admin := models.User{
		Username:              username,
		Role:                  "admin",
		IsActive:              true,
		PasswordResetRequired: password_reset_required,
	}
	if err := admin.SetPassword(password); err != nil {
		log.Printf("❌ 创建默认管理员失败：%v", err)
		return
	}
	if err := DB.Create(&admin).Error; err != nil {
		log.Printf("❌ 创建默认管理员失败：%v", err)
		return
	}
	log.Printf("✅ 默认管理员已创建：%s", admin.Username)
}

func seedDefaultSettings() {
	// NOTE:
	// - FirstOrCreate 在多实例同时启动时会产生竞态：SELECT 未看到记录，随后 INSERT 冲突。
	// - 用 ON CONFLICT DO NOTHING 保证幂等并避免启动时刷错日志。
	defaultSettings := []models.Setting{
		{Key: "default_trial_days", Value: "7"},
		{Key: "registration_mode", Value: "open"},
		{Key: "notify_group_link", Value: ""},
		{Key: "email_verification", Value: "false"},
		{Key: "stripe_allowed_payment_methods", Value: ""},
		{Key: "telegram_approval_admin_ids", Value: ""},
	}

	for _, s := range defaultSettings {
		if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&s).Error; err != nil {
			log.Printf("⚠️  初始化 %s 失败：%v", s.Key, err)
		}
	}
}

func seedDefaultPlanGroups() {
	var defaultCount int64
	if err := DB.Model(&models.PlanGroup{}).Where(`"is_default" = ?`, true).Count(&defaultCount).Error; err != nil {
		log.Printf("⚠️  检查默认套餐分组失败：%v", err)
		return
	}
	if defaultCount > 0 {
		return
	}

	var group models.PlanGroup
	if err := DB.Where("key = ?", "DEFAULT").First(&group).Error; err == nil {
		if err := DB.Model(&models.PlanGroup{}).Where("key = ?", group.Key).Update(`"is_default"`, true).Error; err != nil {
			log.Printf("⚠️  修复默认套餐分组失败：%v", err)
		}
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("⚠️  查询默认套餐分组失败：%v", err)
		return
	}

	group = models.PlanGroup{
		Key:         "DEFAULT",
		Name:        "默认分组",
		Description: "系统默认套餐分组",
		IsDefault:   true,
		SortOrder:   10,
	}
	if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&group).Error; err != nil {
		log.Printf("⚠️  初始化套餐分组 %s 失败：%v", group.Key, err)
	}
}

func seedDefaultPlanGroupEmbyPolicyTemplates() {
	var groups []models.PlanGroup
	if err := DB.Find(&groups).Error; err != nil {
		log.Printf("⚠️  查询套餐分组权益模板失败：%v", err)
		return
	}
	for _, group := range groups {
		template := models.PlanGroupEmbyPolicyTemplate{
			PlanGroupKey:                   group.Key,
			SimultaneousStreamLimit:        3,
			EnableContentDownloading:       false,
			EnableLiveTvAccess:             false,
			EnableSyncTranscoding:          false,
			EnableAudioPlaybackTranscoding: false,
			EnableVideoPlaybackTranscoding: false,
			EnablePlaybackRemuxing:         true,
			EnableRemoteAccess:             true,
		}
		if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&template).Error; err != nil {
			log.Printf("⚠️  初始化套餐分组权益模板失败：planGroup=%s err=%v", group.Key, err)
		}
	}
}

// Close 关闭数据库连接
func Close() error {
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
