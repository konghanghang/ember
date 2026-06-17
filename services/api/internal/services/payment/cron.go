package payment

import (
	"context"

	"github.com/konghang/ember/backend/internal/db"
)

var expirePendingPaymentsStore = expirePendingPaymentsWithDB

// ExpirePendingPayments 将超时的 pending 支付单推进到 expired 状态。
func ExpirePendingPayments(ctx context.Context) (int64, error) {
	return expirePendingPaymentsStore(ctx)
}

// expirePendingPaymentsWithDB runs the production SQL update for expired pending payments.
func expirePendingPaymentsWithDB(ctx context.Context) (int64, error) {
	result := db.DB.WithContext(ctx).Exec(
		`UPDATE payments SET status='expired', "updated_at"=now() WHERE status='pending' AND "expires_at" < now()`)
	return result.RowsAffected, result.Error
}
