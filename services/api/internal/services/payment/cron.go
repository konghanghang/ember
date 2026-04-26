package payment

import (
	"context"

	"github.com/konghang/ember/backend/internal/db"
)

// ExpirePendingPayments 将超时的 pending 支付单推进到 expired 状态。
func ExpirePendingPayments(ctx context.Context) (int64, error) {
	result := db.DB.WithContext(ctx).Exec(
		`UPDATE payments SET status='expired', "updatedAt"=now() WHERE status='pending' AND "expiresAt" < now()`)
	return result.RowsAffected, result.Error
}
