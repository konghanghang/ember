package db

import "time"

// migrateLockRetryIntervalForTest / migrateLockTimeoutForTest /
// setMigrateLockTimingsForTest 仅供同包测试压缩 advisory lock 重试窗口，
// 避免单测耗在真实 30s 超时上。
//
// 单独放进 export_test.go 是 Go 的惯用做法：测试可见，但不进入正式 build。

func migrateLockRetryIntervalForTest() time.Duration { return migrateLockRetryInterval }

func migrateLockTimeoutForTest() time.Duration { return migrateLockTimeout }

func setMigrateLockTimingsForTest(retry, timeout time.Duration) {
	migrateLockRetryInterval = retry
	migrateLockTimeout = timeout
}

// setSchemaFingerprintsForTest 临时替换 fingerprint 列与索引清单，返回还原闭包。
//
// 用于覆盖混合模式分支的测试：fingerprint 列表是 db.go 的全局私有变量，测试需要
// 临时追加新条目（模拟未来发版引入新 fingerprint 但 baseline 漏同步）；测试结束后
// 必须用返回的闭包还原，避免污染其它 test。
func setSchemaFingerprintsForTest(cols []schemaFingerprintColumn, idxs []schemaFingerprintIndex) func() {
	prevCols := schemaFingerprintColumns
	prevIdxs := schemaFingerprintIndexes
	schemaFingerprintColumns = cols
	schemaFingerprintIndexes = idxs
	return func() {
		schemaFingerprintColumns = prevCols
		schemaFingerprintIndexes = prevIdxs
	}
}
