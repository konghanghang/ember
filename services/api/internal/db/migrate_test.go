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

// TestRunMigrate_Mixed_PartialForwardPartialBackfill：模拟边界场景——
// 业务核心表已存在 + 新增 fingerprint 部分缺失 + 缺失项关联的 migration 仍在目录顶层
// （例如手工预建表、或老库被部分人工增量覆盖但未写入 schema_migrations）。
// 混合模式下：缺失项的 SQL forward-only 跑掉，其余 backfill。
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

// TestIsBaselineFile：识别函数同时兼容当前格式与未来推荐格式，且不误判普通增量。
func TestIsBaselineFile(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		// 当前生产格式
		{"20260502_00_schema_baseline.sql", true},
		{"20260422_00_schema_baseline.sql", true},
		// 未来推荐格式
		{"00000000_baseline.sql", true},
		{"00000000_baseline_20260820.sql", true},
		// 普通增量不应被误判
		{"20260427_04_bot_pending_reject_message_context.sql", false},
		{"20260504_00_users-emby-id-unique.sql", false},
		{"20260502_00_some_other.sql", false},
		// 名字片段不算
		{"schema_baseline.sql", false},
		{"my_baseline_xxx.sql", false},
		// 错位的零前缀
		{"0000000_baseline.sql", false},
		{"000000000_baseline.sql", false},
		// 错位的扩展名
		{"00000000_baseline.txt", false},
		{"20260502_00_schema_baseline.SQL", false},
	}
	for _, c := range cases {
		if got := isBaselineFile(c.in); got != c.want {
			t.Errorf("isBaselineFile(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestRunMigrate_BaselineRenameOnExistingDB_ShouldNotReexecute：
// 关键回归——老库已记账上一代 baseline 文件名 + 一份增量；目录被压缩成
// 新 baseline + 切点之后的新增量。启动期必须：
//   - 不重复执行新 baseline（baseline SQL 通常含 CREATE TYPE 等不带 IF NOT EXISTS
//     的语句，重复执行会让 API fail-fast）
//   - 把新 baseline 写入 schema_migrations 视为已应用
//   - 真的执行新增量
func TestRunMigrate_BaselineRenameOnExistingDB_ShouldNotReexecute(t *testing.T) {
	dir := writeMigrations(t, map[string]string{
		"00000000_baseline_20260820.sql": "CREATE TYPE media_type AS ENUM ('MOVIE','TV');",
		"20260820_01_some_feature.sql":   "ALTER TABLE users ADD COLUMN beta TEXT;",
	})

	driver := newFakeDriver()
	// 老库已有记账：上一代 baseline + 一份增量；这两份文件已经移到 archive，目录里已不存在
	driver.applied["20260502_00_schema_baseline.sql"] = "old-baseline-checksum"
	driver.applied["20260504_00_users-emby-id-unique.sql"] = "old-increment-checksum"

	if err := runMigrate(driver, dir); err != nil {
		t.Fatalf("runMigrate 不应失败：%v", err)
	}

	// 新 baseline 不应被执行（视为等价 schema 快照）
	for _, fn := range driver.applyCalls {
		if isBaselineFile(fn) {
			t.Fatalf("新 baseline %s 不应被执行：apply=%v", fn, driver.applyCalls)
		}
	}

	// 新 baseline 应被记账（backfill 路径）
	foundBaselineBackfill := false
	for _, fn := range driver.backfillCalls {
		if fn == "00000000_baseline_20260820.sql" {
			foundBaselineBackfill = true
			break
		}
	}
	if !foundBaselineBackfill {
		t.Fatalf("新 baseline 应被 RecordBackfill：backfill=%v", driver.backfillCalls)
	}

	// 新增量必须被真的执行
	wantApply := []string{"20260820_01_some_feature.sql"}
	if !equalSlices(driver.applyCalls, wantApply) {
		t.Fatalf("新增量应被执行：want=%v got=%v", wantApply, driver.applyCalls)
	}

	if !driver.released {
		t.Fatalf("结束前应释放 advisory lock")
	}
}

// TestRunMigrate_MultipleBaselinesCoexist_FailFast：目录里同时存在两份 baseline
// （例如 baseline 压缩时忘了把老 baseline 移到 archive）必须 fail-fast，
// 不允许任何写操作。
func TestRunMigrate_MultipleBaselinesCoexist_FailFast(t *testing.T) {
	dir := writeMigrations(t, map[string]string{
		"20260502_00_schema_baseline.sql": "CREATE TYPE foo AS ENUM ('A');",
		"00000000_baseline_20260820.sql":  "CREATE TYPE foo AS ENUM ('A');",
		"20260820_01_some_feature.sql":    "SELECT 1;",
	})

	driver := newFakeDriver()
	// 用空库视角：tables / applied 都空 → 走 emptyDB 分支
	// 但 baseline 共存防御应该在分支判断之前就 fail-fast

	err := runMigrate(driver, dir)
	if err == nil {
		t.Fatal("多份 baseline 共存时应 fail-fast")
	}
	if !strings.Contains(err.Error(), "baseline") {
		t.Fatalf("error 应明确提示 baseline 多份共存：%v", err)
	}

	// 不应触发任何写操作
	if len(driver.applyCalls)+len(driver.backfillCalls) != 0 {
		t.Fatalf("baseline 共存防御不应触发写操作：apply=%v backfill=%v",
			driver.applyCalls, driver.backfillCalls)
	}
	if !driver.released {
		t.Fatalf("即便 fail-fast 也应释放 advisory lock")
	}
}

// TestRunMigrate_EmptyDBWithBaseline_AppliesBaseline：新空库部署不应被
// baseline 重命名豁免影响——applied 为空 → baseline 必须被真的执行。
// 这个测试是为了防止"baseline 跳过执行"逻辑在新空库上误触发。
func TestRunMigrate_EmptyDBWithBaseline_AppliesBaseline(t *testing.T) {
	dir := writeMigrations(t, map[string]string{
		"00000000_baseline_20260820.sql": "CREATE TABLE users (...);",
		"20260820_01_some_feature.sql":   "ALTER TABLE users ADD COLUMN beta TEXT;",
	})

	driver := newFakeDriver()
	// 新空库：tables / columns / indexes / applied 全部留空

	if err := runMigrate(driver, dir); err != nil {
		t.Fatalf("runMigrate 不应失败：%v", err)
	}

	// 新空库：必须按字典序应用全部 SQL（含 baseline）
	want := []string{"00000000_baseline_20260820.sql", "20260820_01_some_feature.sql"}
	if !equalSlices(driver.applyCalls, want) {
		t.Fatalf("新空库下 baseline 必须被真的执行：want=%v got=%v", want, driver.applyCalls)
	}
	if len(driver.backfillCalls) != 0 {
		t.Fatalf("新空库不应触发 backfill：%v", driver.backfillCalls)
	}
}
