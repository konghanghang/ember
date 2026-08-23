package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

// migrateAdvisoryLockKey 是 PG advisory lock 的固定 key。
//
// 占用，不要复用：本项目其它地方如需 advisory lock，请另选常量。
const migrateAdvisoryLockKey int64 = 6437520014390001

// migrateLockRetryInterval / migrateLockTimeout 控制 advisory lock 抢占的重试节奏。
//
// 用 var 而非 const 是为了让 migrate_test.go 能在不影响生产路径的情况下
// 把窗口压短到毫秒级（见 setMigrateLockTimingsForTest）。
var (
	migrateLockRetryInterval = time.Second
	migrateLockTimeout       = 30 * time.Second
)

// migrateLogPrefix 统一 migration 阶段日志前缀，便于与 API 业务日志区分。
const migrateLogPrefix = "[Migrate] "

// migrateBusinessCoreTables 是用于探测"老库 backfill"的代表性业务表清单。
//
// 故意不复用 db.go 的 tableChecks 全表清单：tableChecks 会随每条新增 SQL 增长，
// 用于 VerifySchema 兜底；本清单只回答"这是不是一个已经在跑业务的库"，越稳定越好。
var migrateBusinessCoreTables = []string{
	"users",
	"plans",
	"subscriptions",
	"settings",
}

// migrationFile 描述一份待处理的 SQL 文件。
type migrationFile struct {
	filename string // 仅文件名（如 20260427_04_xxx.sql）
	abspath  string // 绝对路径
	checksum string // SHA-256 hex（按字节计算，不做行尾规范化）
	content  []byte // 文件原始字节
}

// migrationDriver 把 migrate.go 与具体数据库后端解耦的薄抽象层。
//
// 拆出此接口的唯一目的是让 forward-only / backfill / checksum 三类分支可以在
// 不依赖真实 PG 的情况下被自动化测试覆盖（仓库现在没有 sqlmock / testcontainers
// 基础设施）。生产路径只有一份实现：gormMigrationDriver。
type migrationDriver interface {
	// AcquireAdvisoryLock 用 pg_try_advisory_lock 抢占 migration 的串行锁。
	// 抢到返回 true，未抢到返回 false。
	AcquireAdvisoryLock(key int64) (bool, error)
	// ReleaseAdvisoryLock 释放上面抢到的锁。
	ReleaseAdvisoryLock(key int64) error
	// EnsureMigrationsTable 幂等地建出 schema_migrations 表。
	EnsureMigrationsTable() error
	// HasTable 判断业务表是否存在。
	HasTable(table string) (bool, error)
	// HasColumn 判断列是否存在。
	HasColumn(table, column string) (bool, error)
	// HasIndex 判断索引是否存在。
	HasIndex(table, index string) (bool, error)
	// LoadAppliedMigrations 拉出 schema_migrations 已记账数据。返回 filename → checksum。
	LoadAppliedMigrations() (map[string]string, error)
	// ApplyMigrationInTx 在外层事务里执行 SQL 并写入记账行；
	// SQL 与记账写入要么同时成功，要么同时回滚。
	ApplyMigrationInTx(file migrationFile) error
	// RecordBackfill 仅写入记账行（不执行 SQL），用于老库 backfill 分支；
	// 使用 ON CONFLICT DO NOTHING 保证重入幂等。
	RecordBackfill(file migrationFile) error
}

// Migrate 在 ember api/gateway 启动期被调用，完成所有未应用 SQL 的 forward-only 应用。
//
// 调用时机：紧跟 InitDB 之后、VerifySchema 之前；DB 必须已经初始化。
//
// 关键约束：advisory lock 是 PG session 级锁，必须由获取它的同一条物理连接通过
// pg_advisory_unlock 释放。这里用 DB.Connection 把 runMigrate 的整段逻辑（抢锁、
// 决策、应用 SQL、释放锁）固定在单条物理连接上，避免连接池抢到不同连接导致 unlock
// 落空、锁挂在前一条连接上直到 ConnMaxLifetime（最长 1 小时）才被释放。
func Migrate() error {
	if DB == nil {
		return errors.New("DB 尚未初始化；Migrate 必须在 InitDB 之后调用")
	}
	dir, err := resolveMigrationsDir()
	if err != nil {
		return err
	}
	return DB.Connection(func(tx *gorm.DB) error {
		return runMigrate(newGormMigrationDriver(tx), dir)
	})
}

// runMigrate 是真正实现迁移流程的纯函数，便于自动化测试覆盖三类分支。
func runMigrate(driver migrationDriver, dir string) error {
	logf("使用 SQL migration 目录：%s", dir)

	// 1. 先抢 advisory lock，避免多副本 / 孤悬容器并发执行
	if err := acquireMigrateLock(driver); err != nil {
		return err
	}
	defer func() {
		if err := driver.ReleaseAdvisoryLock(migrateAdvisoryLockKey); err != nil {
			logf("释放 advisory lock 失败：%v（不影响本次结果，下次启动会自然重试）", err)
		}
	}()

	// 2. 建出 schema_migrations 表
	if err := driver.EnsureMigrationsTable(); err != nil {
		return fmt.Errorf("创建 schema_migrations 表失败：%w", err)
	}

	// 3. 列出目录下顶层 .sql 文件并预读 checksum
	files, err := loadMigrationFiles(dir)
	if err != nil {
		return fmt.Errorf("读取 migration 目录失败：%w", err)
	}
	if len(files) == 0 {
		logf("目录 %s 下没有顶层 .sql 文件，跳过", dir)
		return nil
	}

	// 3.5 baseline 唯一性防御：所有分支共用此约束。
	// baseline 表达"等价 schema 快照"语义，目录里同一时刻只能有一份；
	// 多份共存会让升级路径分支判断失去单点真相，必须在启动期 fail-fast。
	if err := detectBaselineCollision(files); err != nil {
		return err
	}

	// 4. 拉出已应用集合
	applied, err := driver.LoadAppliedMigrations()
	if err != nil {
		return fmt.Errorf("加载 schema_migrations 失败：%w", err)
	}

	// 5. 探测当前数据库状态，决定走哪条分支
	branch, missingMigrations, err := decideMigrateBranch(driver, applied, files)
	if err != nil {
		return err
	}

	switch branch {
	case migrateBranchEmptyDB:
		logf("分支：新空库（未检测到既存业务表）→ 按字典序 forward-only 跑全部 %d 份 SQL，"+
			"将从空库一次性初始化 schema", len(files))
		return applyForwardOnly(driver, files, applied)
	case migrateBranchBackfill:
		logf("分支：老库 backfill → 把目录中全部 %d 份 SQL 视为已应用灌入 schema_migrations（不执行 SQL）", len(files))
		return backfillAll(driver, files)
	case migrateBranchMixed:
		logf("分支：混合模式 → forward-only 应用 %d 份缺失 fingerprint 的 SQL，backfill %d 份其余 SQL",
			len(missingMigrations), len(files)-len(missingMigrations))
		return applyMixed(driver, files, missingMigrations)
	case migrateBranchOldButMisaligned:
		return errors.New("该库不是干净状态（业务表存在但部分 fingerprint 列/索引缺失），" +
			"且部分缺失项关联的 migration 文件不在 infrastructure/database/ 顶层（通常意味着 schema 真的漂了，" +
			"不是新增 SQL 漏同步）；请按 infrastructure/database/README.md 手工对齐 schema 后再启动 API")
	case migrateBranchForwardOnly:
		logf("分支：正常 forward-only → 已记账 %d 份，目录共 %d 份，按需补齐", len(applied), len(files))
		return applyForwardOnly(driver, files, applied)
	default:
		return fmt.Errorf("未知 migration 分支：%v", branch)
	}
}

// migrateBranch 表示启动期分支判断的几种结果。
type migrateBranch int

const (
	migrateBranchUnknown migrateBranch = iota
	migrateBranchEmptyDB
	migrateBranchBackfill
	migrateBranchOldButMisaligned
	migrateBranchForwardOnly
	// migrateBranchMixed 用于覆盖"业务核心表已存在但 fingerprint 不齐"的边界场景：
	// 例如手工预建了部分业务表、或老库已被部分人工增量覆盖但未写入 schema_migrations。
	// 此时业务核心表存在 + 部分 fingerprint 缺失 + 缺失项对应的 migration 仍在目录顶层，
	// 走 forward-only 跑缺失的、backfill 其余的，保证"一条命令完成升级"承诺。
	migrateBranchMixed
)

// decideMigrateBranch 按"启动期分支判断"表格选支。
//
// 返回值 missingMigrations 仅在 migrateBranchMixed 分支下有意义，记录需要走
// forward-only 的 SQL 文件名集合（带 .sql 后缀）；其它分支返回 nil。
func decideMigrateBranch(driver migrationDriver, applied map[string]string, dirFiles []migrationFile) (migrateBranch, map[string]bool, error) {
	if len(applied) > 0 {
		return migrateBranchForwardOnly, nil, nil
	}

	// schema_migrations 为空：判断业务核心表是否存在
	coreExists, err := businessCoreTablesExist(driver)
	if err != nil {
		return migrateBranchUnknown, nil, err
	}
	if !coreExists {
		return migrateBranchEmptyDB, nil, nil
	}

	// 老库：检查 fingerprint 列/索引是否齐
	aligned, missingDescriptions, missingFingerprintMigrations, err := fingerprintAligned(driver)
	if err != nil {
		return migrateBranchUnknown, nil, err
	}
	if aligned {
		return migrateBranchBackfill, nil, nil
	}

	logf("fingerprint 缺失项：%s", strings.Join(missingDescriptions, ", "))

	// 缺失项关联的 migration 是否都还在目录顶层？
	dirSet := make(map[string]bool, len(dirFiles))
	for _, f := range dirFiles {
		dirSet[f.filename] = true
	}
	allInDir := true
	for m := range missingFingerprintMigrations {
		if !dirSet[m] {
			allInDir = false
			break
		}
	}
	if allInDir {
		// 走混合模式：缺失项的 SQL forward-only 跑掉，其余 backfill。
		return migrateBranchMixed, missingFingerprintMigrations, nil
	}
	return migrateBranchOldButMisaligned, nil, nil
}

// businessCoreTablesExist 判断"业务核心表"是否齐全。
//
// 全部存在才算"老库"。任何一张缺失都视作新空库，避免半建库被误判为已运行。
func businessCoreTablesExist(driver migrationDriver) (bool, error) {
	for _, t := range migrateBusinessCoreTables {
		ok, err := driver.HasTable(t)
		if err != nil {
			return false, fmt.Errorf("探测业务核心表 %s 失败：%w", t, err)
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

// fingerprintAligned 判断 schemaFingerprintColumns / schemaFingerprintIndexes
// 中所有列与索引在数据库中是否齐全。
//
// 返回三段信息：是否齐全、缺失项的人类可读描述、缺失项关联的 migration 文件名集合
// （带 .sql 后缀）。混合模式分支需要"关联 migration 文件名集合"作 forward-only 子集。
//
// 故意复用 db.go 同包私有变量，避免双份维护漂移。
func fingerprintAligned(driver migrationDriver) (bool, []string, map[string]bool, error) {
	var missingDescriptions []string
	missingMigrations := make(map[string]bool)
	for _, c := range schemaFingerprintColumns {
		ok, err := driver.HasColumn(c.table, c.column)
		if err != nil {
			return false, nil, nil, fmt.Errorf("探测列 %s.%s 失败：%w", c.table, c.column, err)
		}
		if !ok {
			missingDescriptions = append(missingDescriptions, fmt.Sprintf("%s.%s（来自 %s）", c.table, c.column, c.migration))
			missingMigrations[c.migration+".sql"] = true
		}
	}
	for _, c := range schemaFingerprintIndexes {
		ok, err := driver.HasIndex(c.table, c.index)
		if err != nil {
			return false, nil, nil, fmt.Errorf("探测索引 %s.%s 失败：%w", c.table, c.index, err)
		}
		if !ok {
			missingDescriptions = append(missingDescriptions, fmt.Sprintf("%s.%s（来自 %s）", c.table, c.index, c.migration))
			missingMigrations[c.migration+".sql"] = true
		}
	}
	return len(missingDescriptions) == 0, missingDescriptions, missingMigrations, nil
}

// applyForwardOnly 在 forward-only 分支处理一批文件。
//
// 三种分支：已应用 + checksum 一致 → 跳过；已应用但 checksum 不一致 → fail-fast；
// 未应用 → 在外层事务内执行 SQL + 同一事务写入记账行。
//
// baseline 重命名豁免：当 schema_migrations 已有任意记账（说明这是已经跑过 v1.4.x
// 的库），目录里出现的"未应用"baseline 文件视为"等价 schema 快照"，仅记账不执行。
// 否则会在老库上重新跑 baseline 里 CREATE TYPE 等不带 IF NOT EXISTS 的 DDL，
// 直接 fail-fast 让 API 起不来。新空库分支调用此函数时 applied 必然为空，不会
// 进入这条分支，baseline 会按字典序被真的执行。
func applyForwardOnly(driver migrationDriver, files []migrationFile, applied map[string]string) error {
	var executed, baselineBackfilled int
	hasPriorAccounting := len(applied) > 0
	for _, f := range files {
		if existing, ok := applied[f.filename]; ok {
			if existing != f.checksum {
				return fmt.Errorf("checksum 不一致：%s（数据库记录 %s，当前文件 %s）；"+
					"已应用 SQL 不允许改写，请先确认行尾是否被改成 CRLF（仓库根 .gitattributes 强制 LF），"+
					"否则只能新增前向 SQL 抵消错误效果。"+
					"本地开发环境如确认无业务数据可重置：手动 `DROP TABLE schema_migrations` 后重启 API 会重新进入 backfill 分支。",
					f.filename, existing, f.checksum)
			}
			continue
		}
		// baseline 重命名豁免：老库下出现的"未应用"baseline 是上一次压缩切点之后
		// 重命名而来的等价快照，不该被执行。
		if hasPriorAccounting && isBaselineFile(f.filename) {
			logf("→ baseline %s 视为等价 schema 快照（老库已有记账），仅记账不执行", f.filename)
			if err := driver.RecordBackfill(f); err != nil {
				return fmt.Errorf("baseline 记账 %s 失败：%w", f.filename, err)
			}
			baselineBackfilled++
			continue
		}
		logf("→ 应用 %s", f.filename)
		if err := driver.ApplyMigrationInTx(f); err != nil {
			return fmt.Errorf("执行 %s 失败：%w", f.filename, err)
		}
		executed++
	}
	if baselineBackfilled > 0 {
		logf("forward-only 完成：本次实际执行 %d 份 SQL，baseline 仅记账 %d 份", executed, baselineBackfilled)
	} else {
		logf("forward-only 完成：本次实际执行 %d 份 SQL", executed)
	}
	return nil
}

// backfillAll 把目录中全部 SQL 标记为"已应用"灌入 schema_migrations，不执行 SQL。
func backfillAll(driver migrationDriver, files []migrationFile) error {
	var inserted int
	for _, f := range files {
		if err := driver.RecordBackfill(f); err != nil {
			return fmt.Errorf("backfill 写入 %s 失败：%w", f.filename, err)
		}
		inserted++
	}
	logf("backfill 完成：%d 份 SQL 已标记为已应用", inserted)
	return nil
}

// applyMixed 实现混合模式：missingMigrations 中的 SQL 走 forward-only（执行 + 记账），
// 其余 SQL 仅 backfill 记账。文件按字典序遍历，与其它分支保持一致。
func applyMixed(driver migrationDriver, files []migrationFile, missingMigrations map[string]bool) error {
	var executed, backfilled int
	for _, f := range files {
		if missingMigrations[f.filename] {
			logf("→ 应用 %s（混合模式：fingerprint 缺失）", f.filename)
			if err := driver.ApplyMigrationInTx(f); err != nil {
				return fmt.Errorf("执行 %s 失败：%w", f.filename, err)
			}
			executed++
			continue
		}
		if err := driver.RecordBackfill(f); err != nil {
			return fmt.Errorf("backfill 写入 %s 失败：%w", f.filename, err)
		}
		backfilled++
	}
	logf("混合模式完成：实际执行 %d 份 SQL，backfill %d 份", executed, backfilled)
	return nil
}

// acquireMigrateLock 抢占 advisory lock，30s 超时窗口内每秒重试一次。
func acquireMigrateLock(driver migrationDriver) error {
	deadline := time.Now().Add(migrateLockTimeout)
	attempt := 0
	for {
		attempt++
		ok, err := driver.AcquireAdvisoryLock(migrateAdvisoryLockKey)
		if err != nil {
			return fmt.Errorf("尝试 advisory lock 失败：%w", err)
		}
		if ok {
			if attempt > 1 {
				logf("advisory lock 在第 %d 次尝试时抢到", attempt)
			}
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("advisory lock 抢占超时（30s）；如果确认无并发，" +
				"`docker compose ps -a` 排查是否有孤悬的 ember-api 容器后重启")
		}
		time.Sleep(migrateLockRetryInterval)
	}
}

// loadMigrationFiles 列出顶层 .sql 文件并按字典序返回，预读 checksum 与内容。
func loadMigrationFiles(dir string) ([]migrationFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []migrationFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(e.Name()), ".sql") {
			continue
		}
		abs := filepath.Join(dir, e.Name())
		content, err := os.ReadFile(abs)
		if err != nil {
			return nil, fmt.Errorf("读取 %s 失败：%w", e.Name(), err)
		}
		files = append(files, migrationFile{
			filename: e.Name(),
			abspath:  abs,
			checksum: sha256Hex(content),
			content:  content,
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].filename < files[j].filename })
	return files, nil
}

// sha256Hex 计算字节内容的 SHA-256 hex；
// 不做行尾规范化（依赖仓库根 .gitattributes 强制 LF 保证跨平台稳定）。
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// baselineFilenamePattern 识别"等价 schema 快照"类型的 baseline 文件。
//
// 同时兼容两种命名格式：
//   - 当前推荐格式：00000000_baseline_YYYYMMDD.sql 或 00000000_baseline.sql
//     （全 0 前缀让 baseline 永远字典序最先，且文件名一眼可辨"我是 baseline"）
//   - 历史命名格式：YYYYMMDD_NN_schema_baseline.sql
//     （v1.4.0 baseline 起步使用过的格式，现行 baseline 已迁移到全 0 命名；
//     保留识别是为了让仍记账着旧文件名的老库重启时能干净走重命名豁免路径）
var baselineFilenamePattern = regexp.MustCompile(
	`^(?:[0-9]{8}_[0-9]{2}_schema_baseline|0{8}_baseline(?:_[0-9]{8})?)\.sql$`,
)

// isBaselineFile 判断文件名是否符合 baseline 命名约定。
func isBaselineFile(filename string) bool {
	return baselineFilenamePattern.MatchString(filename)
}

// detectBaselineCollision 探测目录里是否同时存在多份 baseline 文件。
//
// baseline 表达"等价 schema 快照"，目录里同一时刻必须只有一份；多份共存
// 会让升级路径分支判断失去单点真相（新空库部署时按字典序会先后跑两份 baseline
// 触发幂等性不足的语句失败），必须在启动期 fail-fast。
func detectBaselineCollision(files []migrationFile) error {
	var baselines []string
	for _, f := range files {
		if isBaselineFile(f.filename) {
			baselines = append(baselines, f.filename)
		}
	}
	if len(baselines) > 1 {
		return fmt.Errorf("目录中同时存在多份 baseline 文件：%s；"+
			"baseline 表达'等价 schema 快照'语义，目录里只能有一份。"+
			"做 baseline 压缩时请把老 baseline 移到 archive/pre-<date>/ 后再启动",
			strings.Join(baselines, ", "))
	}
	return nil
}

// resolveMigrationsDir 决定 SQL migration 文件所在目录。
//
// 优先级：EMBER_MIGRATIONS_DIR > 几个常见相对路径（按本地 go run 工作目录由近及远）。
// 容器内默认值 /app/migrations 由镜像 ENV 注入。
func resolveMigrationsDir() (string, error) {
	if v := strings.TrimSpace(os.Getenv("EMBER_MIGRATIONS_DIR")); v != "" {
		abs, err := filepath.Abs(v)
		if err != nil {
			return "", fmt.Errorf("EMBER_MIGRATIONS_DIR 无法转为绝对路径：%w", err)
		}
		return abs, nil
	}
	candidates := []string{
		"../../infrastructure/database",
		"../infrastructure/database",
		"infrastructure/database",
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			abs, _ := filepath.Abs(p)
			return abs, nil
		}
	}
	return "", errors.New("未找到 SQL migration 目录；" +
		"请通过 EMBER_MIGRATIONS_DIR 显式指定（一般是仓库根的 infrastructure/database）")
}

// logf 统一加 [Migrate] 前缀输出。
func logf(format string, args ...any) {
	log.Printf(migrateLogPrefix+format, args...)
}

// ============== gormMigrationDriver ==============

// gormMigrationDriver 是 migrationDriver 的生产实现，基于现有 *gorm.DB。
//
// 关键约束：本 driver 拿到的 *gorm.DB 必须由 DB.Connection 包装（即所有调用都跑
// 在同一条物理连接上），否则 advisory lock 的 acquire / release 会落到不同连接，
// release 静默失败，锁悬挂直到 ConnMaxLifetime。Migrate() 已保证这一点。
type gormMigrationDriver struct {
	db *gorm.DB
}

func newGormMigrationDriver(db *gorm.DB) *gormMigrationDriver {
	return &gormMigrationDriver{db: db}
}

func (g *gormMigrationDriver) AcquireAdvisoryLock(key int64) (bool, error) {
	var ok bool
	if err := g.db.Raw("SELECT pg_try_advisory_lock(?)", key).Scan(&ok).Error; err != nil {
		return false, err
	}
	return ok, nil
}

func (g *gormMigrationDriver) ReleaseAdvisoryLock(key int64) error {
	// pg_advisory_unlock 返回 bool：true = 本会话曾持有并已释放；false = 本会话不持有该锁。
	// 取出返回值显式判断：连接亲和性失效时记 warning 但不返回 error，
	// 释放失败不应该让 Migrate 整体失败（PG 会随会话结束自然释放，最坏情况是
	// 单条连接在 ConnMaxLifetime 内仍持有锁）。
	var released bool
	if err := g.db.Raw("SELECT pg_advisory_unlock(?)", key).Scan(&released).Error; err != nil {
		return err
	}
	if !released {
		logf("pg_advisory_unlock 返回 false：锁不在本会话；通常是连接亲和性失效，请人工排查")
	}
	return nil
}

func (g *gormMigrationDriver) EnsureMigrationsTable() error {
	const ddl = `CREATE TABLE IF NOT EXISTS schema_migrations (
    filename TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    checksum TEXT NOT NULL
)`
	return g.db.Exec(ddl).Error
}

func (g *gormMigrationDriver) HasTable(table string) (bool, error) {
	return g.db.Migrator().HasTable(table), nil
}

func (g *gormMigrationDriver) HasColumn(table, column string) (bool, error) {
	// GORM Migrator HasColumn 在传字符串表名时支持 information_schema 查询。
	return g.db.Migrator().HasColumn(table, column), nil
}

func (g *gormMigrationDriver) HasIndex(table, index string) (bool, error) {
	// GORM Migrator HasIndex 在 PG 上会查 pg_indexes。
	return g.db.Migrator().HasIndex(table, index), nil
}

func (g *gormMigrationDriver) LoadAppliedMigrations() (map[string]string, error) {
	type row struct {
		Filename string
		Checksum string
	}
	var rows []row
	if err := g.db.Raw("SELECT filename, checksum FROM schema_migrations").Scan(&rows).Error; err != nil {
		return nil, err
	}
	m := make(map[string]string, len(rows))
	for _, r := range rows {
		m[r.Filename] = r.Checksum
	}
	return m, nil
}

func (g *gormMigrationDriver) ApplyMigrationInTx(file migrationFile) error {
	// 外层包裹事务：SQL 与记账行同生死。SQL 文件本身不允许写 BEGIN/COMMIT
	// （顶层 SQL 约定，README 已说明）；DO $$ ... $$ 块在 PG 视为单语句，仍可用。
	return g.db.Transaction(func(tx *gorm.DB) error {
		if err := execMigrationSQL(tx.Statement.Context, tx.Statement.ConnPool, string(file.content)); err != nil {
			return err
		}
		return tx.Exec(
			"INSERT INTO schema_migrations (filename, checksum) VALUES (?, ?)",
			file.filename, file.checksum,
		).Error
	})
}

// execMigrationSQL 直接在真实 ConnPool 上执行迁移 SQL，显式绕开 GORM 的
// PreparedStmt 包装层。
//
// 原因：migration 顶层 SQL（尤其 baseline）天然会包含多条 DDL 语句；而项目全局
// 开启了 PrepareStmt 后，事务里的 Exec 会先对整份 SQL 做 PrepareContext。PG 对
// 多语句 prepared statement 支持很差，空库首启时会在 Migrate 阶段直接炸。
// 业务查询仍保留 prepared statement 缓存，只有 migration 执行点走原始 ConnPool。
func execMigrationSQL(ctx context.Context, conn gorm.ConnPool, sql string) error {
	rawConn := unwrapPreparedConnPool(conn)
	_, err := rawConn.ExecContext(ctx, sql)
	return err
}

// unwrapPreparedConnPool 递归剥掉 GORM PrepareStmt 包装，拿到真实的底层 ConnPool。
//
// 可能遇到的两层包装：
//   - *gorm.PreparedStmtDB：非事务路径的全局 prepared statement cache
//   - *gorm.PreparedStmtTX：事务路径对单条物理连接的 prepared statement 包装
//
// migration 执行点必须显式解包，否则多语句 baseline 会先走 PrepareContext。
func unwrapPreparedConnPool(conn gorm.ConnPool) gorm.ConnPool {
	switch t := conn.(type) {
	case *gorm.PreparedStmtTX:
		return unwrapPreparedConnPool(t.Tx)
	case *gorm.PreparedStmtDB:
		return unwrapPreparedConnPool(t.ConnPool)
	default:
		return conn
	}
}

func (g *gormMigrationDriver) RecordBackfill(file migrationFile) error {
	// 老库 backfill：只写记账行，不执行 SQL；ON CONFLICT DO NOTHING 保证重入幂等。
	return g.db.Exec(
		"INSERT INTO schema_migrations (filename, checksum) VALUES (?, ?) ON CONFLICT (filename) DO NOTHING",
		file.filename, file.checksum,
	).Error
}
