// Package account 跨业务的账号生命周期收口设施。
//
// 当前提供 EmbyCompensation：批次 2 把支付履约 / 兑换码 / 注册回滚等链路对 Emby 的
// 写操作从业务事务中剥离，事务外异步调用失败时入队，由 cron `@every 10m` 拉取重试。
//
// 不放在 services/payment 或 services/redemption 是因为补偿链路被 3 个独立业务复用，
// 抽到独立包避免循环引用。
package account

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	policypkg "github.com/konghang/ember/backend/internal/services/policy"
	"gorm.io/gorm"
)

// 拉取与重试参数。
const (
	defaultBatchLimit  = 50
	maxRetries         = 6
	maxLastErrorLength = 500
)

// 指数退避序列：30s / 2m / 10m / 1h / 6h / 24h，超出退化为最后一档。
var backoffSchedule = []time.Duration{
	30 * time.Second,
	2 * time.Minute,
	10 * time.Minute,
	1 * time.Hour,
	6 * time.Hour,
	24 * time.Hour,
}

// EmbyCompensation 提供入队 + cron 处理两个入口。
type EmbyCompensation struct {
	embyService     *embyint.EmbyService
	now             func() time.Time
	createOp        func(context.Context, models.FailedEmbyAsyncOp) error
	updateOpFailure func(context.Context, string, map[string]interface{}) error
}

// NewEmbyCompensation 装配补偿器；embyService 不能为空，便于测试时替换为 mock。
func NewEmbyCompensation(embyService *embyint.EmbyService) *EmbyCompensation {
	if embyService == nil {
		embyService = embyint.GetSharedService()
	}
	return &EmbyCompensation{
		embyService:     embyService,
		now:             func() time.Time { return time.Now().UTC() },
		createOp:        createFailedEmbyAsyncOp,
		updateOpFailure: updateFailedEmbyAsyncOpFailure,
	}
}

// EnsureUnbanned 立刻按当前本地用户状态重算 Emby Policy；失败时入补偿队列。
//
// 设计：历史调用方传入的是 embyUserId，因此这里先反查本地用户，再走统一
// ApplyEffectiveUserPolicy，不能直接把 IsDisabled 写成 false。
//
// 成功：返回 nil；调用方记成功日志即可。
// 失败：尝试入队，若入队成功返回 nil（视为已交给后台重试，调用方主流程不阻塞）。
//
//	若入队也失败返回 error，调用方按需告警。
func (c *EmbyCompensation) EnsureUnbanned(ctx context.Context, op models.FailedEmbyAsyncOp) error {
	op.Action = models.FailedEmbyActionUnban
	if op.OriginRefID == "" {
		op.OriginRefID = op.EmbyUserID
	}
	if err := c.applyEffectivePolicyByEmbyID(op.EmbyUserID, "compensation_unban"); err == nil {
		log.Printf("[Compensation] 同步重算 Emby Policy 成功 origin=%s originRefId=%s embyUserId=%s",
			op.Origin, op.OriginRefID, op.EmbyUserID)
		return nil
	} else {
		log.Printf("[Compensation] 同步重算 Emby Policy 失败，入补偿队列 origin=%s originRefId=%s embyUserId=%s err=%v",
			op.Origin, op.OriginRefID, op.EmbyUserID, err)
	}
	return c.Enqueue(ctx, op)
}

// EnsureDeleted 立刻尝试删除 Emby 用户；失败时入补偿队列。
// 用于注册回滚 / 后台创建失败回滚链路。
func (c *EmbyCompensation) EnsureDeleted(ctx context.Context, op models.FailedEmbyAsyncOp) error {
	op.Action = models.FailedEmbyActionDelete
	if op.OriginRefID == "" {
		op.OriginRefID = op.EmbyUserID
	}
	if err := c.embyService.DeleteUser(op.EmbyUserID); err == nil {
		log.Printf("[Compensation] 同步删除 Emby 成功 origin=%s originRefId=%s embyUserId=%s",
			op.Origin, op.OriginRefID, op.EmbyUserID)
		return nil
	} else {
		log.Printf("[Compensation] 同步删除 Emby 失败，入补偿队列 origin=%s originRefId=%s embyUserId=%s err=%v",
			op.Origin, op.OriginRefID, op.EmbyUserID, err)
	}
	return c.Enqueue(ctx, op)
}

// Enqueue 将一条待补偿操作写入 failed_emby_async_ops。调用方在事务外失败时使用。
//
// op.NextAttemptAt 留空时按首档退避（30s 后）写入；ID 由模型 BeforeCreate 自动生成。
func (c *EmbyCompensation) Enqueue(ctx context.Context, op models.FailedEmbyAsyncOp) error {
	if strings.TrimSpace(string(op.Origin)) == "" {
		return errors.New("compensation: origin 不能为空")
	}
	if !isValidFailedEmbyOrigin(op.Origin) {
		return fmt.Errorf("compensation: origin 无效: %s", op.Origin)
	}
	if strings.TrimSpace(op.EmbyUserID) == "" {
		return errors.New("compensation: embyUserId 不能为空")
	}
	if strings.TrimSpace(string(op.Action)) == "" {
		return errors.New("compensation: action 不能为空")
	}
	if !isValidFailedEmbyAction(op.Action) {
		return fmt.Errorf("compensation: action 无效: %s", op.Action)
	}
	if op.NextAttemptAt.IsZero() {
		op.NextAttemptAt = c.now().Add(backoffSchedule[0])
	}

	if err := c.createOp(ctx, op); err != nil {
		return fmt.Errorf("compensation: 入队失败 origin=%s embyUserId=%s err=%w", op.Origin, op.EmbyUserID, err)
	}
	log.Printf("[Compensation] 入队 origin=%s action=%s originRefId=%s embyUserId=%s nextAttemptAt=%s",
		op.Origin, op.Action, op.OriginRefID, op.EmbyUserID, op.NextAttemptAt.Format(time.RFC3339))
	return nil
}

// createFailedEmbyAsyncOp 将补偿操作写入数据库，作为 Enqueue 的默认持久化实现。
func createFailedEmbyAsyncOp(ctx context.Context, op models.FailedEmbyAsyncOp) error {
	return db.DB.WithContext(ctx).Create(&op).Error
}

func isValidFailedEmbyOrigin(origin models.FailedEmbyAsyncOpOrigin) bool {
	switch origin {
	case models.FailedEmbyOriginPaymentUnban,
		models.FailedEmbyOriginRedemptionUnban,
		models.FailedEmbyOriginRegisterCleanup:
		return true
	default:
		return false
	}
}

func isValidFailedEmbyAction(action models.FailedEmbyAsyncOpAction) bool {
	switch action {
	case models.FailedEmbyActionUnban, models.FailedEmbyActionDelete:
		return true
	default:
		return false
	}
}

// ProcessResult 描述单轮处理结果，便于 cron 与测试观测。
type ProcessResult struct {
	Processed int // 本轮成功并删除的条目数
	Failed    int // 本轮执行失败需要继续退避的条目数
}

// Process 执行一轮补偿处理；幂等，可被 cron 反复调用。返回值用于运维观测。
//
// 处理策略：
//   - 按 nextAttemptAt <= now() 顺序拉取最多 defaultBatchLimit 条
//   - 成功删除该行
//   - 失败 retries++ + 指数退避；retries > maxRetries 时仍保留行，但写 ERROR 日志告警
func (c *EmbyCompensation) Process(ctx context.Context) (ProcessResult, error) {
	now := c.now()
	var ops []models.FailedEmbyAsyncOp
	if err := db.DB.WithContext(ctx).
		Where(`"next_attempt_at" <= ?`, now).
		Order(`"next_attempt_at" ASC`).
		Limit(defaultBatchLimit).
		Find(&ops).Error; err != nil {
		return ProcessResult{}, fmt.Errorf("compensation: 拉取队列失败 err=%w", err)
	}

	if len(ops) == 0 {
		return ProcessResult{}, nil
	}

	result := ProcessResult{}
	for i := range ops {
		op := ops[i]
		if err := c.execute(ctx, op); err != nil {
			c.markFailure(ctx, op, err)
			result.Failed++
			continue
		}
		if delErr := db.DB.WithContext(ctx).Delete(&models.FailedEmbyAsyncOp{}, "id = ?", op.ID).Error; delErr != nil {
			log.Printf("[Compensation] 成功执行后删除队列失败 id=%s err=%v", op.ID, delErr)
			result.Failed++
			continue
		}
		log.Printf("[Compensation] 成功 origin=%s action=%s originRefId=%s embyUserId=%s retries=%d",
			op.Origin, op.Action, op.OriginRefID, op.EmbyUserID, op.Retries)
		result.Processed++
	}
	return result, nil
}

// execute 执行单条补偿操作；按 origin + action 路由到 emby service。
func (c *EmbyCompensation) execute(_ context.Context, op models.FailedEmbyAsyncOp) error {
	switch op.Action {
	case models.FailedEmbyActionUnban:
		return c.applyEffectivePolicyByEmbyID(op.EmbyUserID, "compensation_retry_unban")
	case models.FailedEmbyActionDelete:
		return c.embyService.DeleteUser(op.EmbyUserID)
	default:
		return fmt.Errorf("compensation: 未知 action=%s id=%s", op.Action, op.ID)
	}
}

func (c *EmbyCompensation) applyEffectivePolicyByEmbyID(embyUserID, reason string) error {
	var user models.User
	if err := db.DB.Where("emby_id = ?", strings.TrimSpace(embyUserID)).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("compensation: 找不到 embyUserId 对应用户 embyUserId=%s", embyUserID)
		}
		return err
	}
	return policypkg.NewService(c.embyService).ApplyEffectiveUserPolicy(user.ID, reason)
}

// markFailure 把失败记录写回，更新退避时间与错误信息；超过 maxRetries 写 ERROR 日志告警。
func (c *EmbyCompensation) markFailure(ctx context.Context, op models.FailedEmbyAsyncOp, execErr error) {
	retries := op.Retries + 1
	delay := backoffSchedule[len(backoffSchedule)-1]
	if retries-1 < len(backoffSchedule) {
		delay = backoffSchedule[retries-1]
	}
	nextAt := c.now().Add(delay)
	errMsg := truncate(execErr.Error(), maxLastErrorLength)

	updates := map[string]interface{}{
		"retries":         retries,
		"next_attempt_at": nextAt,
		"last_error":      errMsg,
		"updated_at":      c.now(),
	}
	if err := c.updateOpFailure(ctx, op.ID, updates); err != nil {
		log.Printf("[Compensation] 写回失败状态错误 id=%s err=%v", op.ID, err)
		return
	}

	if retries > maxRetries {
		log.Printf("[Compensation] 重试超限 origin=%s action=%s originRefId=%s embyUserId=%s retries=%d lastError=%s",
			op.Origin, op.Action, op.OriginRefID, op.EmbyUserID, retries, errMsg)
		return
	}
	log.Printf("[Compensation] 失败退避 origin=%s action=%s originRefId=%s embyUserId=%s retries=%d nextAttemptAt=%s err=%s",
		op.Origin, op.Action, op.OriginRefID, op.EmbyUserID, retries, nextAt.Format(time.RFC3339), errMsg)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// updateFailedEmbyAsyncOpFailure 写回失败补偿操作的重试状态。
func updateFailedEmbyAsyncOpFailure(ctx context.Context, id string, updates map[string]interface{}) error {
	return db.DB.WithContext(ctx).
		Model(&models.FailedEmbyAsyncOp{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// 兜底：防止 gorm.DB 在某些场景为 nil（例如部分测试构造）。
var _ = gorm.ErrRecordNotFound
