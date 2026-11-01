package directplay

import (
	"github.com/konghang/ember/backend/internal/services/p115account"
	"github.com/konghang/ember/backend/internal/services/p115quota"
)

// CompatibilityTransferLimits applies only to GET-based compatibility without
// explicit or successfully matched playback-event evidence.
type CompatibilityTransferLimits struct {
	UserHourly, UserDaily, AccountHourly, AccountDaily int
}

// TransferAuthorization is re-evaluated inside the content lock. A nil set of
// sublimits means stronger playback evidence; the plan total always applies.
type TransferAuthorization struct {
	Allowed             bool
	CompatibilityLimits *CompatibilityTransferLimits
}

// compatibilityQuotaScopes isolates user and Provider-account budgets without
// exposing raw Provider identities or changing existing user usage counters.
func (service *Service) compatibilityQuotaScopes(quota *transferQuotaContext, playback p115account.ActiveAccountCredential, limits *CompatibilityTransferLimits) ([]p115quota.TransferQuotaScope, error) {
	if limits == nil {
		return nil, nil
	}
	if quota == nil || service.keyDeriver == nil {
		return nil, ErrRedisUnavailable
	}
	key, err := service.keyDeriver.PlaybackAccountKey(playback.ProviderUserID)
	if err != nil {
		return nil, ErrInvalidRequest
	}
	return []p115quota.TransferQuotaScope{
		{ID: "compat-user-" + quota.UserID, HourlyLimit: limits.UserHourly, DailyLimit: limits.UserDaily},
		{ID: "compat-account-" + key, HourlyLimit: limits.AccountHourly, DailyLimit: limits.AccountDaily},
	}, nil
}

// releaseTransferReservations refunds the total and every reserved sublimit;
// individual Redis failures retain their original pending expiry.
func (service *Service) releaseTransferReservations(quota *transferQuotaContext, attemptID string) {
	service.releaseTransferReservation(quota.UserID, quota.UserID, attemptID)
	for _, scope := range quota.AdditionalScopes {
		service.releaseTransferReservation(quota.UserID, scope.ID, attemptID)
	}
}
