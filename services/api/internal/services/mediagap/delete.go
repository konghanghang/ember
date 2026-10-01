package mediagap

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DeleteClosedGaps 原子删除最多 100 条已入库/已忽略工单；锁定后重验状态，不触发下载或媒体删除。
// 缺失、未收口或任一写入失败时整批回滚。删除忽略记录也会删除其扫描抑制依据。
func (s *Service) DeleteClosedGaps(ctx context.Context, ids []string) (int64, error) {
	if len(ids) == 0 || len(ids) > 100 {
		return 0, ErrMediaGapDeleteIDs
	}
	unique := make([]string, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			return 0, ErrMediaGapDeleteIDs
		}
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}
	// 固定加锁顺序，避免两个交叉批次以相反顺序锁定相同行。
	slices.Sort(unique)
	err := db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var gaps []models.MediaGap
		if err := tx.Select("id", "status").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", unique).Order("id ASC").Find(&gaps).Error; err != nil {
			return fmt.Errorf("查询待删除工单失败: %w", err)
		}
		if len(gaps) != len(unique) {
			return ErrMediaGapNotFound
		}
		for _, gap := range gaps {
			if gap.Status != models.MediaGapStatusIngested && gap.Status != models.MediaGapStatusIgnored {
				return ErrMediaGapDeleteState
			}
		}
		result := tx.Where("id IN ? AND status IN ?", unique, []models.MediaGapStatus{models.MediaGapStatusIngested, models.MediaGapStatusIgnored}).Delete(&models.MediaGap{})
		if result.Error != nil {
			return fmt.Errorf("删除缺集工单失败: %w", result.Error)
		}
		if result.RowsAffected != int64(len(unique)) {
			return ErrMediaGapStateConflict
		}
		return nil
	})
	if err != nil {
		log.Printf("[MediaGap] 删除工单未完成 count=%d firstId=%s err=%v", len(unique), unique[0], err)
		return 0, err
	}
	log.Printf("[MediaGap] 已删除收口工单 count=%d ids=%v", len(unique), unique)
	return int64(len(unique)), nil
}
