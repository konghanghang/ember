package db

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeDriver 是 migrationDriver 的可控实现，用于覆盖三类分支的关键路径。
//
// 设计思路：直接在 map 里维护"已应用"集合，不模拟事务/SQL 执行细节；
// 因为 migrate.go 的逻辑边界恰好就是"什么时候执行 SQL、什么时候只记账、
// 什么时候 fail-fast"，这部分用 fake 完全可控。
type fakeDriver struct {
	// 抢锁返回值：默认 true；测试可改成 false 模拟锁忙
	lockOK   bool
	lockErr  error
	released bool

	// 业务表 / 列 / 索引存在性
	tables  map[string]bool
	columns map[string]bool // key 是 "table.column"
	indexes map[string]bool // key 是 "table.index"

	// 已应用集合：filename → checksum
	applied map[string]string

	// 实际触发记录：用于断言
	applyCalls    []string // ApplyMigrationInTx 调用过的 filename
	backfillCalls []string // RecordBackfill 调用过的 filename

	// applyShouldFail 模拟 SQL 执行失败：true 时 ApplyMigrationInTx 返回 error
	// 且不写 applied / 不追加 applyCalls（与外层事务回滚后什么都没发生的语义对齐）。
	applyShouldFail bool
}

func newFakeDriver() *fakeDriver {
	return &fakeDriver{
		lockOK:  true,
		tables:  map[string]bool{},
		columns: map[string]bool{},
		indexes: map[string]bool{},
		applied: map[string]string{},
	}
}

func (f *fakeDriver) AcquireAdvisoryLock(_ int64) (bool, error) {
	return f.lockOK, f.lockErr
}

func (f *fakeDriver) ReleaseAdvisoryLock(_ int64) error {
	f.released = true
	return nil
}

func (f *fakeDriver) EnsureMigrationsTable() error {
	return nil
}

func (f *fakeDriver) HasTable(table string) (bool, error) {
	return f.tables[table], nil
}

func (f *fakeDriver) HasColumn(table, column string) (bool, error) {
	return f.columns[table+"."+column], nil
}

func (f *fakeDriver) HasIndex(table, index string) (bool, error) {
	return f.indexes[table+"."+index], nil
}

func (f *fakeDriver) LoadAppliedMigrations() (map[string]string, error) {
	out := make(map[string]string, len(f.applied))
	for k, v := range f.applied {
		out[k] = v
	}
	return out, nil
}

func (f *fakeDriver) ApplyMigrationInTx(file migrationFile) error {
	if f.applyShouldFail {
		return errors.New("simulated apply failure")
	}
	f.applyCalls = append(f.applyCalls, file.filename)
	f.applied[file.filename] = file.checksum
	return nil
}

func (f *fakeDriver) RecordBackfill(file migrationFile) error {
	f.backfillCalls = append(f.backfillCalls, file.filename)
	if _, ok := f.applied[file.filename]; !ok {
		f.applied[file.filename] = file.checksum
	}
	return nil
}

// markBusinessCoreTablesPresent 模拟"老库"业务核心表全部存在的状态。
func (f *fakeDriver) markBusinessCoreTablesPresent() {
	for _, t := range migrateBusinessCoreTables {
		f.tables[t] = true
	}
}

// markFingerprintAligned 模拟所有 fingerprint 列与索引齐全。
func (f *fakeDriver) markFingerprintAligned() {
	for _, c := range schemaFingerprintColumns {
		f.columns[c.table+"."+c.column] = true
	}
	for _, c := range schemaFingerprintIndexes {
		f.indexes[c.table+"."+c.index] = true
	}
}

// writeMigrations 在临时目录写入若干 SQL 文件并返回目录。
func writeMigrations(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("写入 %s 失败：%v", name, err)
		}
	}
	return dir
}

// TestRunMigrate_ForwardOnly_OnlyAppliesMissing：已应用一份，目录共三份，
// runMigrate 只跑剩下两份；释放锁、不混用 backfill。
func TestRunMigrate_ForwardOnly_OnlyAppliesMissing(t *testing.T) {
	dir := writeMigrations(t, map[string]string{
		"20260101_01_a.sql": "SELECT 1;",
		"20260102_01_b.sql": "SELECT 2;",
		"20260103_01_c.sql": "SELECT 3;",
	})

	driver := newFakeDriver()
	// schema_migrations 已有第一份，预置正确 checksum 避免误触 checksum 不一致
	firstContent, _ := os.ReadFile(filepath.Join(dir, "20260101_01_a.sql"))
	driver.applied["20260101_01_a.sql"] = sha256Hex(firstContent)

	if err := runMigrate(driver, dir); err != nil {
		t.Fatalf("runMigrate 不应失败：%v", err)
	}

	// 应只跑两份，且按字典序
	want := []string{"20260102_01_b.sql", "20260103_01_c.sql"}
	if !equalSlices(driver.applyCalls, want) {
		t.Fatalf("ApplyMigrationInTx 调用集不符：want=%v got=%v", want, driver.applyCalls)
	}
	if len(driver.backfillCalls) != 0 {
		t.Fatalf("forward-only 分支不应触发 backfill：got=%v", driver.backfillCalls)
	}
	if !driver.released {
		t.Fatalf("结束前应释放 advisory lock")
	}
}

// TestRunMigrate_Backfill_OldDBWithFingerprintAligned：业务核心表存在 +
// schema_migrations 为空 + fingerprint 齐 → 全部灌入记账，不实际执行 SQL。
func TestRunMigrate_Backfill_OldDBWithFingerprintAligned(t *testing.T) {
	dir := writeMigrations(t, map[string]string{
		"20260101_01_a.sql": "DROP TABLE shouldnt_be_executed;",
		"20260102_01_b.sql": "DROP TABLE shouldnt_be_executed_either;",
	})

	driver := newFakeDriver()
	driver.markBusinessCoreTablesPresent()
	driver.markFingerprintAligned()
	// schema_migrations 留空 → 走 backfill 分支

	if err := runMigrate(driver, dir); err != nil {
		t.Fatalf("runMigrate 不应失败：%v", err)
	}

	if len(driver.applyCalls) != 0 {
		t.Fatalf("backfill 分支不应实际执行 SQL：got=%v", driver.applyCalls)
	}
	wantBackfill := []string{"20260101_01_a.sql", "20260102_01_b.sql"}
	gotBackfill := append([]string(nil), driver.backfillCalls...)
	// backfill 不强制顺序，但 runMigrate 内部按字典序遍历，这里直接比较有序。
	if !equalSlices(gotBackfill, wantBackfill) {
		t.Fatalf("RecordBackfill 调用集不符：want=%v got=%v", wantBackfill, gotBackfill)
	}
	if !driver.released {
		t.Fatalf("结束前应释放 advisory lock")
	}
}

// TestRunMigrate_OldButMisaligned_FailFast：业务核心表存在但 fingerprint 缺失 →
// 直接 fail-fast，不应触发 apply 或 backfill。
func TestRunMigrate_OldButMisaligned_FailFast(t *testing.T) {
	dir := writeMigrations(t, map[string]string{
		"20260101_01_a.sql": "SELECT 1;",
	})

	driver := newFakeDriver()
	driver.markBusinessCoreTablesPresent()
	// 故意只补一部分 fingerprint，留一项缺失
	for _, c := range schemaFingerprintColumns {
		driver.columns[c.table+"."+c.column] = true
	}
	// indexes 全部缺失 → 触发 misaligned

	err := runMigrate(driver, dir)
	if err == nil {
		t.Fatal("misaligned 分支应返回 error")
	}
	if !strings.Contains(err.Error(), "干净状态") {
		t.Fatalf("error 应提示老库不对齐：%v", err)
	}
	if len(driver.applyCalls)+len(driver.backfillCalls) != 0 {
		t.Fatalf("misaligned 分支不应触发任何写操作：apply=%v backfill=%v",
			driver.applyCalls, driver.backfillCalls)
	}
	if !driver.released {
		t.Fatalf("即便 fail-fast 也应释放 advisory lock")
	}
}

// TestRunMigrate_ChecksumMismatch_FailFast：磁盘文件被改写后 checksum 与
// schema_migrations 记录不一致 → 报错并指向具体文件。
func TestRunMigrate_ChecksumMismatch_FailFast(t *testing.T) {
	dir := writeMigrations(t, map[string]string{
		"20260101_01_a.sql": "SELECT 1;",
	})

	driver := newFakeDriver()
	// 故意写入与磁盘不同的 checksum 模拟"已应用 SQL 被改写"
	driver.applied["20260101_01_a.sql"] = "deadbeef"

	err := runMigrate(driver, dir)
	if err == nil {
		t.Fatal("checksum 不一致应返回 error")
	}
	if !strings.Contains(err.Error(), "20260101_01_a.sql") {
		t.Fatalf("error 应包含具体文件名：%v", err)
	}
	if !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("error 应明确说明 checksum 不一致：%v", err)
	}
	if len(driver.applyCalls) != 0 {
		t.Fatalf("checksum 不一致应直接 fail-fast，不该触发 apply：%v", driver.applyCalls)
	}
	if !driver.released {
		t.Fatalf("即便 fail-fast 也应释放 advisory lock")
	}
}

// TestRunMigrate_EmptyDB_AppliesAll：业务核心表不存在 + schema_migrations 为空 →
// 按字典序 forward-only 跑全部 SQL。
func TestRunMigrate_EmptyDB_AppliesAll(t *testing.T) {
	dir := writeMigrations(t, map[string]string{
		"20260102_01_b.sql": "SELECT 2;",
		"20260101_01_a.sql": "SELECT 1;",
	})

	driver := newFakeDriver()
	// tables / columns / indexes 全部留空 → 新空库

	if err := runMigrate(driver, dir); err != nil {
		t.Fatalf("runMigrate 不应失败：%v", err)
	}
	want := []string{"20260101_01_a.sql", "20260102_01_b.sql"}
	if !equalSlices(driver.applyCalls, want) {
		t.Fatalf("空库分支应按字典序应用全部 SQL：want=%v got=%v", want, driver.applyCalls)
	}
	if len(driver.backfillCalls) != 0 {
		t.Fatalf("空库分支不应触发 backfill：%v", driver.backfillCalls)
	}
}

// TestAcquireMigrateLock_Timeout：模拟一直抢不到锁，runMigrate 在 advisory
// lock 抢占阶段应当 fail-fast 而不是死循环。
func TestAcquireMigrateLock_Timeout(t *testing.T) {
	dir := writeMigrations(t, map[string]string{
		"20260101_01_a.sql": "SELECT 1;",
	})

	driver := newFakeDriver()
	driver.lockOK = false

	// 把超时窗口与重试间隔暂时缩短，避免单测耗时
	origInterval := migrateLockRetryIntervalForTest()
	origTimeout := migrateLockTimeoutForTest()
	setMigrateLockTimingsForTest(time.Millisecond, 5*time.Millisecond)
	defer setMigrateLockTimingsForTest(origInterval, origTimeout)

	err := runMigrate(driver, dir)
	if err == nil {
		t.Fatal("锁一直抢不到时应返回 error")
	}
	if !strings.Contains(err.Error(), "advisory lock") {
		t.Fatalf("error 应明确指出 advisory lock 抢占超时：%v", err)
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestRunMigrate_ApplyFailure_NoAccountingWritten：SQL 执行失败时记账行不应写入，
// 即便失败也要释放 advisory lock，确保 P1-3 修复后下次启动能重试。
func TestRunMigrate_ApplyFailure_NoAccountingWritten(t *testing.T) {
	dir := writeMigrations(t, map[string]string{
		"20260101_01_a.sql": "SELECT 1;",
	})

	driver := newFakeDriver()
	driver.applyShouldFail = true
	// tables / columns / indexes 全部留空 → 新空库分支，会调 ApplyMigrationInTx

	err := runMigrate(driver, dir)
	if err == nil {
		t.Fatal("ApplyMigrationInTx 失败时应返回 error")
	}
	if len(driver.applied) != 0 {
		t.Fatalf("apply 失败时记账行不应写入：%v", driver.applied)
	}
	if len(driver.applyCalls) != 0 {
		t.Fatalf("apply 失败时不应进入成功分支记录 applyCalls：%v", driver.applyCalls)
	}
	if !driver.released {
		t.Fatalf("即便 apply 失败也应释放 advisory lock")
	}
}

// TestRunMigrate_Mixed_PartialForwardPartialBackfill：模拟未来发版场景——
// 新增顶层 SQL + fingerprint 列表追加，但 baseline 漏同步导致新空库 PG initdb 后
// fingerprint 部分缺失。混合模式下：缺失项的 SQL forward-only 跑掉，其余 backfill。
func TestRunMigrate_Mixed_PartialForwardPartialBackfill(t *testing.T) {
	dir := writeMigrations(t, map[string]string{
		"20260101_01_a.sql": "SELECT 1;",
		"20260601_01_new_feature.sql": "ALTER TABLE users ADD COLUMN something_new TEXT;",
	})

	// 临时替换 fingerprint 列表：仅留一条新增项指向 20260601_01_new_feature。
	// 这样老库 + fingerprint 缺该列 + 该 migration 在目录中 → 触发混合模式。
	restore := setSchemaFingerprintsForTest(
		[]schemaFingerprintColumn{
			{"users", "something_new", "20260601_01_new_feature"},
		},
		nil, // 索引清单清空，避免 markFingerprintAligned 干扰
	)
	defer restore()

	driver := newFakeDriver()
	driver.markBusinessCoreTablesPresent()
	// 故意不补 users.something_new → fingerprint 不齐，但缺失项的 migration 在目录中
	// schema_migrations 留空 → 进入老库判定

	if err := runMigrate(driver, dir); err != nil {
		t.Fatalf("runMigrate 不应失败：%v", err)
	}

	// 期望：新增项 SQL 走 ApplyMigrationInTx，旧 SQL 走 RecordBackfill
	wantApply := []string{"20260601_01_new_feature.sql"}
	if !equalSlices(driver.applyCalls, wantApply) {
		t.Fatalf("混合模式 ApplyMigrationInTx 调用集不符：want=%v got=%v", wantApply, driver.applyCalls)
	}
	wantBackfill := []string{"20260101_01_a.sql"}
	if !equalSlices(driver.backfillCalls, wantBackfill) {
		t.Fatalf("混合模式 RecordBackfill 调用集不符：want=%v got=%v", wantBackfill, driver.backfillCalls)
	}
	if !driver.released {
		t.Fatalf("结束前应释放 advisory lock")
	}
}
