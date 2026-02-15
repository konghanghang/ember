package db

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// InitDB 初始化数据库连接
func InitDB() {
	// 尝试加载 .env 文件（多个可能的位置）
	envPaths := []string{
		".env",              // 当前目录
		"../../.env",        // 项目根目录
		"../../../.env",     // 以防万一
		"services/api/.env", // 从根目录运行时
	}

	envLoaded := false
	for _, path := range envPaths {
		if err := godotenv.Load(path); err == nil {
			log.Printf("✅ 成功加载环境变量：%s", path)
			envLoaded = true
			break
		}
	}

	if !envLoaded {
		log.Println("⚠️  警告：无法从文件加载 .env，将使用系统环境变量")
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("❌ DATABASE_URL 环境变量未设置")
	}

	// 创建自定义 logger，显示详细的 SQL 日志
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
		logger.Config{
			SlowThreshold:             time.Second, // 慢查询阈值
			LogLevel:                  logger.Info, // 日志级别：Info 显示所有 SQL
			IgnoreRecordNotFoundError: false,       // 不忽略 RecordNotFound 错误
			Colorful:                  true,        // 彩色输出
		},
	)

	var err error
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
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(10 * time.Minute)

	// 测试连接
	if err := sqlDB.Ping(); err != nil {
		log.Fatalf("❌ 数据库无法 ping 通：%v", err)
	}

	// 获取 PostgreSQL 版本
	var pgVersion string
	DB.Raw("SHOW server_version").Scan(&pgVersion)
	fmt.Printf("✅ PostgreSQL 版本：%s\n", pgVersion)

	// 按需自动迁移表结构
	if os.Getenv("AUTO_MIGRATE") == "true" {
		if err := AutoMigrate(); err != nil {
			log.Fatalf("❌ 数据库迁移失败：%v", err)
		}
		log.Println("✅ 数据库迁移完成")
	} else {
		log.Println("ℹ️  AUTO_MIGRATE 未启用，跳过数据库迁移")
	}

	// 初始化默认管理员
	seedDefaultAdmin()
	seedDefaultSettings()

	fmt.Println("✅ 数据库连接成功")
}

// seedDefaultAdmin 初始化默认管理员账号
func seedDefaultAdmin() {
	var count int64
	DB.Model(&models.User{}).Where("role = ?", "admin").Count(&count)
	if count > 0 {
		return
	}

	username := os.Getenv("ADMIN_USERNAME")
	password := os.Getenv("ADMIN_PASSWORD")
	if username == "" || password == "" {
		log.Println("⚠️  跳过 admin 初始化：ADMIN_USERNAME 或 ADMIN_PASSWORD 未设置")
		return
	}

	admin := models.User{
		Username: username,
		Role:     "admin",
		IsActive: true,
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
	}

	for _, s := range defaultSettings {
		if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&s).Error; err != nil {
			log.Printf("⚠️  初始化 %s 失败：%v", s.Key, err)
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

// AutoMigrate 手动执行数据库迁移（可选）
// 仅在确认需要时调用
func AutoMigrate() error {
	if err := DB.AutoMigrate(
		&models.RedemptionCode{},
		&models.Redemption{},
		&models.Setting{},
		&models.User{},
		&models.Subscription{},
		&models.PlaybackRanking{},
	); err != nil {
		return err
	}

	if DB.Migrator().HasTable("invites") {
		if err := DB.Migrator().DropTable("invites"); err != nil {
			return err
		}
	}

	return nil
}
