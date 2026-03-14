package services

import paymentpkg "github.com/konghang/ember/backend/internal/services/payment"

type PaymentService = paymentpkg.PaymentService
type CreatePlanRequest = paymentpkg.CreatePlanRequest
type UpdatePlanRequest = paymentpkg.UpdatePlanRequest
type GetPlansRequest = paymentpkg.GetPlansRequest
type GetPlansResponse = paymentpkg.GetPlansResponse
type CreateCheckoutRequest = paymentpkg.CreateCheckoutRequest
type CreateCheckoutResponse = paymentpkg.CreateCheckoutResponse
type PaymentWithMeta = paymentpkg.PaymentWithMeta
type GetPaymentsRequest = paymentpkg.GetPaymentsRequest
type GetPaymentsResponse = paymentpkg.GetPaymentsResponse

var (
	ErrPlanNotFound        = paymentpkg.ErrPlanNotFound
	ErrPaymentFailed       = paymentpkg.ErrPaymentFailed
	ErrStripeNotConfigured = paymentpkg.ErrStripeNotConfigured
)

func NewPaymentService() *PaymentService {
	return paymentpkg.NewPaymentService()
}
