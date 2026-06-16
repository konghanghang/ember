package device

import (
	"context"
	"fmt"

	"github.com/konghang/ember/backend/internal/db"
)

// CleanupOldDeviceActions 清理 retainDays 天前的 device_actions 记录。
func CleanupOldDeviceActions(ctx context.Context, retainDays int) (int64, error) {
	if retainDays <= 0 {
		return 0, fmt.Errorf("retainDays must be positive")
	}
	result := db.DB.WithContext(ctx).Exec(
		fmt.Sprintf(`DELETE FROM device_actions WHERE "created_at" < now() - interval '%d days'`, retainDays))
	return result.RowsAffected, result.Error
}
