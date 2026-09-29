package playback

import (
	"fmt"
	"log"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// persistRankingBatch 以周期唯一的批次领取发布权，并在同一事务中完整写入明细。
// 返回 false 表示该周期已经存在；空榜仍写入批次，任何明细错误都回滚本期。
func persistRankingBatch(result *RankingComputeResult) (bool, error) {
	if result == nil || result.BatchID == "" {
		return false, fmt.Errorf("排行榜批次未初始化")
	}
	batch := models.PlaybackRankingBatch{
		ID: result.BatchID, Period: result.Period, PeriodStart: result.Start,
		PeriodEnd: result.End, SnapshotAt: result.ComputedAt, TotalDuration: &result.TotalDuration,
	}
	rankings := make([]models.PlaybackRanking, 0, len(result.Movies)+len(result.Episodes))
	rankings = append(rankings, result.Movies...)
	rankings = append(rankings, result.Episodes...)
	for i := range rankings {
		rankings[i].BatchID = batch.ID
		rankings[i].Period = batch.Period
		rankings[i].SnapshotAt = batch.SnapshotAt
		rankings[i].PeriodStart = batch.PeriodStart
		rankings[i].PeriodEnd = batch.PeriodEnd
	}

	created := false
	err := db.DB.Transaction(func(tx *gorm.DB) error {
		insert := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "period"}, {Name: "period_start"}, {Name: "period_end"}},
			DoNothing: true,
		}).Create(&batch)
		if insert.Error != nil {
			return insert.Error
		}
		if insert.RowsAffected == 0 {
			return nil
		}
		if len(rankings) > 0 {
			details := tx.Create(&rankings)
			if details.Error != nil {
				return details.Error
			}
			if details.RowsAffected != int64(len(rankings)) {
				return fmt.Errorf("排行榜明细写入不完整: expected=%d actual=%d", len(rankings), details.RowsAffected)
			}
		}
		created = true
		return nil
	})
	if err != nil {
		log.Printf("[PlaybackRanking] persist failed batchId=%s period=%s items=%d err=%v", batch.ID, batch.Period, len(rankings), err)
		return false, err
	}
	log.Printf("[PlaybackRanking] persist done batchId=%s period=%s items=%d created=%v", batch.ID, batch.Period, len(rankings), created)
	return created, nil
}
