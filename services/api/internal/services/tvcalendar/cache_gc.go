package tvcalendar

import (
	"context"

	"github.com/konghang/ember/backend/internal/db"
)

// RunTMDBCacheGC 删除超过 7 天的已过期 TMDB 缓存条目。
func RunTMDBCacheGC(ctx context.Context) (int64, error) {
	result := db.DB.WithContext(ctx).Exec(
		`DELETE FROM tmdb_cache WHERE "expiresAt" < now() - interval '7 days'`)
	return result.RowsAffected, result.Error
}
