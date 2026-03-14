package services

import subscriptionpkg "github.com/konghang/ember/backend/internal/services/subscription"

type SubscriptionService = subscriptionpkg.SubscriptionService
type CreateSubscriptionRequest = subscriptionpkg.CreateSubscriptionRequest

var (
	ErrSubscriptionDuplicated = subscriptionpkg.ErrSubscriptionDuplicated
	ErrSubscriptionNotFound   = subscriptionpkg.ErrSubscriptionNotFound
)

func NewSubscriptionService() *SubscriptionService {
	return subscriptionpkg.NewSubscriptionService()
}
