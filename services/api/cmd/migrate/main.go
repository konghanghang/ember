// migrate 工具：本地空库一次性应用与生产同源的 SQL migration。
//
// 使用方式：
//
//	cd services/api && go run ./cmd/migrate
//
// 也可以从仓库根目录跑：
//
//	go run ./services/api/cmd/migrate
//
// 默认按以下顺序查找 SQL 目录：
//
//	$EMBER_MIGRATIONS_DIR
//	../../infrastructure/database
//	../infrastructure/database
//	infrastructure/database
//
// 流程：InitDB → 按字典序应用顶层 *.sql → VerifySchema 自检 → Bootstrap
// 与生产同一套 SQL 真相源，避免与 GORM AutoMigrate 漂移。所有 SQL 必须幂等
// （现行规范如此），重复运行不会破坏已有数据。
//
// 严禁在生产环境使用：生产 schema 升级请按 infrastructure/database/README.md 手工执行。
package main

import (
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/konghang/ember/backend/internal/db"
)

func main() {
	db.InitDB()
	defer db.Close()

	migrationsDir := resolveMigrationsDir()
	log.Printf("ℹ️  使用 SQL migration 目录：%s", migrationsDir)

	files, err := loadMigrationFiles(migrationsDir)
	if err != nil {
		log.Fatalf("❌ 读取 migration 失败：%v", err)
	}
	if len(files) == 0 {
		log.Fatalf("❌ migration 目录 %s 下没有顶层 .sql 文件", migrationsDir)
	}

	log.Printf("ℹ️  开始按字典序应用 %d 份 migration ...", len(files))
	for _, f := range files {
		log.Printf("→ 应用 %s", filepath.Base(f))
		sqlBytes, err := os.ReadFile(f)
		if err != nil {
			log.Fatalf("❌ 读取 %s 失败：%v", f, err)
		}
		if err := db.DB.Exec(string(sqlBytes)).Error; err != nil {
			log.Fatalf("❌ 执行 %s 失败：%v", filepath.Base(f), err)
		}
	}
	log.Println("✅ 所有 migration 应用完成")

	log.Println("ℹ️  执行 schema 自检（VerifySchema）...")
	if err := db.VerifySchema(); err != nil {
		log.Fatalf("❌ schema 自检失败：%v", err)
	}

	log.Println("ℹ️  写入默认管理员 / 默认设置 / 默认套餐分组 ...")
	db.Bootstrap()
	log.Println("✅ Bootstrap 完成")
}

// resolveMigrationsDir 决定 SQL migration 文件所在目录。
//
// 优先级：EMBER_MIGRATIONS_DIR > 几个常见相对路径。命中后返回绝对路径。
func resolveMigrationsDir() string {
	if v := strings.TrimSpace(os.Getenv("EMBER_MIGRATIONS_DIR")); v != "" {
		abs, err := filepath.Abs(v)
		if err != nil {
			log.Fatalf("❌ EMBER_MIGRATIONS_DIR 无法转为绝对路径：%v", err)
		}
		return abs
	}
	candidates := []string{
		"../../infrastructure/database",
		"../infrastructure/database",
		"infrastructure/database",
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			abs, _ := filepath.Abs(p)
			return abs
		}
	}
	log.Fatal("❌ 未找到 SQL migration 目录；请通过 EMBER_MIGRATIONS_DIR 显式指定（一般是仓库根的 infrastructure/database）")
	return ""
}

// loadMigrationFiles 列出顶层 .sql 文件并按字典序返回绝对路径。
//
// 不递归到子目录，archive/ 不参与执行。
func loadMigrationFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(e.Name()), ".sql") {
			continue
		}
		files = append(files, filepath.Join(dir, e.Name()))
	}
	sort.Strings(files)
	return files, nil
}
