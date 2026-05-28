package policy

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	defaultPolicySyncWorkerLimit     = 20
	defaultPolicySyncProcessingTTL   = 15 * time.Minute
	policySyncProcessingRecoveryText = "processing 超时回收，等待重试"
)

// EmbyPolicySyncWorkerResult 汇总一次 worker 执行中回收、领取和完成的任务数量。
type EmbyPolicySyncWorkerResult struct {
	Recovered int
	Claimed   int
	Succeeded int
	Failed    int
}

// EnqueueUserPolicySyncRetry 记录单个用户的待重试 Policy 同步任务。
// 当同一用户已有 pending/processing 任务时直接复用现有任务，避免破坏 active 唯一约束。
func (s *Service) EnqueueUserPolicySyncRetry(user *models.User, reason string, cause error) error {
	if s == nil || s.db == nil {
		return errors.New("Policy 服务未配置数据库")
	}
	task, err := buildUserPolicySyncRetryTask(user, reason, cause, time.Now().UTC())
	if err != nil || task == nil {
		return err
	}
	var activeCount int64
	if err := s.db.Model(&models.EmbyPolicySyncTask{}).
		Where("user_id = ? AND status IN ?", task.UserID, []string{SyncStatusPending, SyncStatusProcessing}).
		Count(&activeCount).Error; err != nil {
		return err
	}
	if activeCount > 0 {
		return nil
	}
	return s.db.Create(task).Error
}

// buildUserPolicySyncRetryTask 把注册后首次同步失败转换成 worker 可继续消费的 pending 任务。
func buildUserPolicySyncRetryTask(user *models.User, reason string, cause error, now time.Time) (*models.EmbyPolicySyncTask, error) {
	if user == nil {
		return nil, errors.New("用户不能为空")
	}
	if strings.TrimSpace(user.EmbyID) == "" {
		return nil, nil
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "unspecified"
	}
	planGroupKey := ""
	if user.PlanGroup != nil {
		planGroupKey = *user.PlanGroup
	}
	msg := truncateError(cause)
	return &models.EmbyPolicySyncTask{
		UserID:       user.ID,
		EmbyID:       user.EmbyID,
		PlanGroupKey: planGroupKey,
		Reason:       reason,
		Status:       SyncStatusPending,
		Attempts:     1,
		LastError:    &msg,
		NextRetryAt:  &now,
	}, nil
}

// ProcessPendingEmbyPolicySyncTasks 回收超时 processing 任务，并领取到期 pending 任务执行 Emby Policy 同步。
func (s *Service) ProcessPendingEmbyPolicySyncTasks(ctx context.Context, limit int) (*EmbyPolicySyncWorkerResult, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("Policy 服务未配置数据库")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if limit <= 0 {
		limit = defaultPolicySyncWorkerLimit
	}
	result := &EmbyPolicySyncWorkerResult{}

	recovered, recoveredBatchIDs, err := s.recoverStalePolicySyncTasks(ctx, defaultPolicySyncProcessingTTL)
	if err != nil {
		return nil, err
	}
	result.Recovered = recovered
	if err := s.refreshBatchIDs(recoveredBatchIDs); err != nil {
		return nil, err
	}

	tasks, err := s.claimPendingPolicySyncTasks(ctx, limit)
	if err != nil {
		return nil, err
	}
	result.Claimed = len(tasks)
	if len(tasks) == 0 {
		return result, nil
	}

	claimedBatchIDs := batchIDsFromTasks(tasks)
	if err := s.refreshBatchIDs(claimedBatchIDs); err != nil {
		return nil, err
	}

	for _, task := range tasks {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if err := s.ApplyEffectiveUserPolicy(task.UserID, task.Reason); err != nil {
			result.Failed++
			if updateErr := s.finishPolicySyncTask(ctx, task, SyncStatusFailed, err); updateErr != nil {
				return result, updateErr
			}
			log.Printf("[PolicyWorker] 用户 Emby Policy 同步失败: batchID=%s taskID=%s userID=%s reason=%s err=%v", stringValue(task.BatchID), task.ID, task.UserID, task.Reason, err)
		} else {
			result.Succeeded++
			if updateErr := s.finishPolicySyncTask(ctx, task, SyncStatusSynced, nil); updateErr != nil {
				return result, updateErr
			}
		}
		if task.BatchID != nil {
			if err := s.refreshBatchStatus(*task.BatchID); err != nil {
				return result, err
			}
		}
	}

	return result, nil
}

// claimPendingPolicySyncTasks 在事务中锁定到期 pending 任务并标记为 processing。
func (s *Service) claimPendingPolicySyncTasks(ctx context.Context, limit int) ([]models.EmbyPolicySyncTask, error) {
	var tasks []models.EmbyPolicySyncTask
	now := time.Now().UTC()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND (next_retry_at IS NULL OR next_retry_at <= ?)", SyncStatusPending, now).
			Order("next_retry_at ASC NULLS FIRST, created_at ASC").
			Limit(limit).
			Find(&tasks).Error; err != nil {
			return err
		}
		if len(tasks) == 0 {
			return nil
		}
		ids := make([]string, 0, len(tasks))
		for _, task := range tasks {
			ids = append(ids, task.ID)
		}
		return tx.Model(&models.EmbyPolicySyncTask{}).
			Where("id IN ?", ids).
			Updates(map[string]any{
				"status":        SyncStatusProcessing,
				"attempts":      gorm.Expr("attempts + 1"),
				"last_error":    nil,
				"next_retry_at": nil,
				"updated_at":    now,
			}).Error
	})
	return tasks, err
}

// finishPolicySyncTask 写入单个 processing 任务的终态和失败原因。
func (s *Service) finishPolicySyncTask(ctx context.Context, task models.EmbyPolicySyncTask, status string, err error) error {
	var lastError *string
	if err != nil {
		msg := truncateError(err)
		lastError = &msg
	}
	return s.db.WithContext(ctx).Model(&models.EmbyPolicySyncTask{}).
		Where("id = ? AND status = ?", task.ID, SyncStatusProcessing).
		Updates(map[string]any{
			"status":        status,
			"last_error":    lastError,
			"next_retry_at": nil,
			"updated_at":    time.Now().UTC(),
		}).Error
}

// recoverStalePolicySyncTasks 将超时 processing 任务退回 pending，避免进程中断后永久阻塞。
func (s *Service) recoverStalePolicySyncTasks(ctx context.Context, timeout time.Duration) (int, []string, error) {
	if timeout <= 0 {
		timeout = defaultPolicySyncProcessingTTL
	}
	cutoff := time.Now().UTC().Add(-timeout)
	var tasks []models.EmbyPolicySyncTask
	if err := s.db.WithContext(ctx).
		Where("status = ? AND updated_at <= ?", SyncStatusProcessing, cutoff).
		Find(&tasks).Error; err != nil {
		return 0, nil, err
	}
	if len(tasks) == 0 {
		return 0, nil, nil
	}
	ids := make([]string, 0, len(tasks))
	batchIDs := batchIDsFromTasks(tasks)
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	now := time.Now().UTC()
	if err := s.db.WithContext(ctx).Model(&models.EmbyPolicySyncTask{}).
		Where("id IN ? AND status = ?", ids, SyncStatusProcessing).
		Updates(map[string]any{
			"status":        SyncStatusPending,
			"attempts":      gorm.Expr("attempts + 1"),
			"last_error":    policySyncProcessingRecoveryText,
			"next_retry_at": now,
			"updated_at":    now,
		}).Error; err != nil {
		return 0, nil, err
	}
	log.Printf("[PolicyWorker] 已回收超时 processing 任务: count=%d timeout=%s", len(tasks), timeout)
	return len(tasks), batchIDs, nil
}

// refreshBatchIDs 按任务变化过的 batch id 重新计算批次摘要字段。
func (s *Service) refreshBatchIDs(batchIDs []string) error {
	for _, batchID := range batchIDs {
		if err := s.refreshBatchStatus(batchID); err != nil {
			return err
		}
	}
	return nil
}

// resolveBatchStatus 根据任务聚合数量计算批次状态和结束时间。
func resolveBatchStatus(total, pending, processing, synced, failed int, currentFinishedAt *time.Time, now time.Time) (string, *time.Time) {
	if total == 0 {
		return SyncStatusSynced, ensureFinishedAt(currentFinishedAt, now)
	}
	if pending == 0 && processing == 0 {
		finishedAt := ensureFinishedAt(currentFinishedAt, now)
		if failed == 0 {
			return SyncStatusSynced, finishedAt
		}
		if synced == 0 {
			return SyncStatusFailed, finishedAt
		}
		return SyncStatusPartialFailed, finishedAt
	}
	if processing > 0 {
		return SyncStatusProcessing, nil
	}
	return SyncStatusPending, nil
}

// ensureFinishedAt 保留已有结束时间；首次进入终态时使用当前时间。
func ensureFinishedAt(current *time.Time, now time.Time) *time.Time {
	if current != nil {
		return current
	}
	return &now
}

// batchIDsFromTasks 从任务列表提取去重后的非空 batch id。
func batchIDsFromTasks(tasks []models.EmbyPolicySyncTask) []string {
	seen := make(map[string]struct{}, len(tasks))
	out := make([]string, 0, len(tasks))
	for _, task := range tasks {
		if task.BatchID == nil || *task.BatchID == "" {
			continue
		}
		if _, ok := seen[*task.BatchID]; ok {
			continue
		}
		seen[*task.BatchID] = struct{}{}
		out = append(out, *task.BatchID)
	}
	return out
}

// stringValue 将可空字符串指针转成日志可用的稳定字符串。
func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
