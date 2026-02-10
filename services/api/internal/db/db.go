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
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// InitDB 初始化数据库连接
// 设计原则：保持与 Prisma 的数据兼容性
func InitDB() {
	// 加载 .env 文件
	if err := godotenv.Load("../.env"); err != nil {
		log.Println("⚠️  警告：无法加载 .env 文件，使用环境变量")
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("❌ DATABASE_URL 环境变量未设置")
	}

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),

		// 使用 UTC 时间（与 Prisma 保持一致）
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

	// ⚠️ 重要：不执行 AutoMigrate
	// 因为表已经由 Prisma 创建，我们只是连接现有数据库
	// 如果需要迁移，使用 prisma migrate deploy

	fmt.Println("✅ 数据库连接成功")
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
	return DB.AutoMigrate(
		&models.Admin{},
		&models.Invite{},
		&models.User{},
		&models.Subscription{},
	)
}
