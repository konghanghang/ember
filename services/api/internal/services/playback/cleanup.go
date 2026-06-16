package playback

import (
	"context"
	"fmt"

	"github.com/konghang/ember/backend/internal/db"
)

// CleanupOldRankings 清理 retainDays 天前的 playback_rankings 记录。
func CleanupOldRankings(ctx context.Context, retainDays int) (int64, error) {
	if retainDays <= 0 {
		return 0, fmt.Errorf("retainDays must be positive")
	}
	result := db.DB.WithContext(ctx).Exec(
		fmt.Sprintf(`DELETE FROM playback_rankings WHERE created_at < now() - interval '%d days'`, retainDays))
	return result.RowsAffected, result.Error
}
