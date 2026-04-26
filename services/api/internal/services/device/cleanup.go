package device

import (
	"context"
	"fmt"

	"github.com/konghang/ember/backend/internal/db"
)

// CleanupOldDeviceActions 清理 retainDays 天前的 device_actions 记录。
func CleanupOldDeviceActions(ctx context.Context, retainDays int) (int64, error) {
	result := db.DB.WithContext(ctx).Exec(
		fmt.Sprintf(`DELETE FROM device_actions WHERE "createdAt" < now() - interval '%d days'`, retainDays))
	return result.RowsAffected, result.Error
}
