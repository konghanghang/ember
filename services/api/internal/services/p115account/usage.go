package p115account

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/services/p115quota"
)

// accountSummary enriches administrator playback summaries with Redis usage
// while preserving null counters when Redis or Provider identity is unavailable.
func (s *Service) accountSummary(ctx context.Context, account *models.P115Account) *AccountSummary {
	summary := accountSummary(account)
	if account == nil || account.Role != models.P115AccountRolePlayback || summary == nil ||
		s.leases == nil || s.keyDeriver == nil || account.ProviderUserID == nil || strings.TrimSpace(*account.ProviderUserID) == "" {
		return summary
	}
	accountKey, err := s.keyDeriver.PlaybackAccountKey(*account.ProviderUserID)
	if err != nil {
		return summary
	}
	usage, err := s.leases.AccountUsage(ctx, accountKey, s.now().UTC())
	if err != nil {
		log.Printf("[P115Quota] playback 用量读取失败 scope=admin accountId=%s errorType=%T", account.ID, err)
		return summary
	}
	available := true
	summary.UsageAvailable = &available
	summary.ReservedStreams = usage.ReservedStreams
	summary.ActiveStreams = usage.ActiveStreams
	summary.OccupiedStreams = usage.OccupiedStreams
	return summary
}

// PersonalUsageSummary is the current user's Redis-backed attribution and
// transfer usage. All pointer counts remain null when Redis is unavailable.
type PersonalUsageSummary struct {
	PlaybackMode        models.P115PlaybackMode `json:"p115PlaybackMode"`
	UsageAvailable      bool                    `json:"usageAvailable"`
	UserReservedStreams *int                    `json:"userReservedStreams"`
	UserActiveStreams   *int                    `json:"userActiveStreams"`
	UserOccupiedStreams *int                    `json:"userOccupiedStreams"`
	TransferPending     *int                    `json:"transferPending"`
	TransferHourlyUsed  *int                    `json:"transferHourlyUsed"`
	TransferHourlyLimit int                     `json:"transferHourlyLimit"`
	TransferDailyUsed   *int                    `json:"transferDailyUsed"`
	TransferDailyLimit  int                     `json:"transferDailyLimit"`
}

// GetPersonalUsage returns user attribution and transfer counts even for a
// system-mode user who has not bound a personal playback account.
func (s *Service) GetPersonalUsage(ctx context.Context, ownerUserID string) (*PersonalUsageSummary, error) {
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID == "" {
		return nil, ErrOwnerUserIDRequired
	}
	policy, err := s.store.GetPersonalPlanPolicy(ctx, ownerUserID)
	if err != nil {
		return nil, err
	}
	return s.personalUsage(ctx, ownerUserID, policy), nil
}

func (s *Service) personalUsage(ctx context.Context, ownerUserID string, policy PersonalPlanPolicy) *PersonalUsageSummary {
	summary := &PersonalUsageSummary{
		PlaybackMode: policy.PlaybackMode, TransferHourlyLimit: policy.TransferHourlyLimit, TransferDailyLimit: policy.TransferDailyLimit,
	}
	if s.leases == nil || s.businessTimezone == nil {
		return summary
	}
	now := s.now().UTC()
	userUsage, err := s.leases.UserUsage(ctx, ownerUserID, now)
	if err != nil {
		log.Printf("[P115Quota] 用户播放用量读取失败 scope=personal_usage userId=%s errorType=%T", ownerUserID, err)
		return summary
	}
	transferStore, ok := s.leases.(interface {
		TransferUsage(context.Context, p115quota.TransferUsageRequest, time.Time) (p115quota.TransferUsage, error)
	})
	if !ok {
		return summary
	}
	dayStart, dayEnd := p115quota.DayWindow(now, s.businessTimezone)
	transferUsage, err := transferStore.TransferUsage(ctx, p115quota.TransferUsageRequest{
		UserID: ownerUserID, DayStart: dayStart, DayEnd: dayEnd,
	}, now)
	if err != nil {
		log.Printf("[P115Quota] 用户转存用量读取失败 scope=personal_usage userId=%s errorType=%T", ownerUserID, err)
		return summary
	}
	summary.UsageAvailable = true
	summary.UserReservedStreams = intPointer(userUsage.ReservedStreams)
	summary.UserActiveStreams = intPointer(userUsage.ActiveStreams)
	summary.UserOccupiedStreams = intPointer(userUsage.OccupiedStreams)
	summary.TransferPending = intPointer(transferUsage.Pending)
	summary.TransferHourlyUsed = intPointer(transferUsage.HourlyUsed)
	summary.TransferDailyUsed = intPointer(transferUsage.DailyUsed)
	return summary
}

// applyPersonalUsage reads account, user and transfer usage as one availability
// unit. Missing Redis keys are real zeroes; any command failure keeps all counts null.
func (s *Service) applyPersonalUsage(ctx context.Context, summary *PersonalAccountSummary, account *models.P115Account, policy PersonalPlanPolicy) {
	if summary == nil || account == nil || s.leases == nil || s.keyDeriver == nil ||
		s.businessTimezone == nil || account.OwnerUserID == nil ||
		account.ProviderUserID == nil || strings.TrimSpace(*account.ProviderUserID) == "" {
		return
	}
	accountKey, err := s.keyDeriver.PlaybackAccountKey(*account.ProviderUserID)
	if err != nil {
		return
	}
	now := s.now().UTC()
	accountUsage, err := s.leases.AccountUsage(ctx, accountKey, now)
	if err != nil {
		log.Printf("[P115Quota] playback 用量读取失败 scope=personal accountId=%s errorType=%T", account.ID, err)
		return
	}
	personalUsage := s.personalUsage(ctx, *account.OwnerUserID, policy)
	if !personalUsage.UsageAvailable {
		return
	}
	summary.UsageAvailable = true
	summary.ReservedStreams = intPointer(accountUsage.ReservedStreams)
	summary.ActiveStreams = intPointer(accountUsage.ActiveStreams)
	summary.OccupiedStreams = intPointer(accountUsage.OccupiedStreams)
	summary.UserReservedStreams = personalUsage.UserReservedStreams
	summary.UserActiveStreams = personalUsage.UserActiveStreams
	summary.UserOccupiedStreams = personalUsage.UserOccupiedStreams
	summary.TransferPending = personalUsage.TransferPending
	summary.TransferHourlyUsed = personalUsage.TransferHourlyUsed
	summary.TransferDailyUsed = personalUsage.TransferDailyUsed
}
