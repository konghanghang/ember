package p115account

import (
	"context"
	"strings"

	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/models"
)

// PersonalPlanPolicy is the exact current plan-group policy used to validate a
// personal account configuration and compute its runtime effective limit.
type PersonalPlanPolicy struct {
	PlanGroupKey            string
	PlaybackMode            models.P115PlaybackMode
	TransferHourlyLimit     int
	TransferDailyLimit      int
	SimultaneousStreamLimit int
}

// UpdatePersonalDirectory resolves an existing directory with the user's
// active credential and conditionally stores the path/ID pair.
func (s *Service) UpdatePersonalDirectory(ctx context.Context, ownerUserID, targetParentPath string) (*PersonalAccountSummary, error) {
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID == "" {
		return nil, ErrOwnerUserIDRequired
	}
	targetParentPath = strings.TrimSpace(targetParentPath)
	if targetParentPath == "" {
		return nil, ErrTargetParentPathRequired
	}
	if s.directoryResolver == nil {
		return nil, ErrDirectoryResolverUnavailable
	}
	account, err := s.store.GetByOwner(ctx, ownerUserID)
	if err != nil {
		return nil, err
	}
	if account.Status != models.P115AccountStatusActive {
		return nil, ErrAccountUnavailable
	}
	ciphertext, appType, userAgent, err := requiredCredentialFields(account)
	if err != nil {
		return nil, err
	}
	cookie, err := s.cipher.Decrypt(ciphertext)
	if err != nil {
		return nil, err
	}
	directory, err := s.directoryResolver.ResolveDirectoryByPath(ctx, p115integration.Credential{
		AccountID: account.ID,
		Cookie:    cookie,
		AppType:   appType,
		UserAgent: userAgent,
	}, p115integration.DirectoryPathQuery{RootID: p115RootDirectoryID, RelativePath: targetParentPath})
	if err != nil {
		return nil, err
	}
	if directory == nil || strings.TrimSpace(directory.ID) == "" || strings.TrimSpace(directory.Path) == "" {
		return nil, p115integration.ErrProviderProtocol
	}
	updated, err := s.store.UpdatePersonalDirectory(ctx, ownerUserID, ciphertext, account.UpdatedAt, strings.TrimSpace(directory.Path), strings.TrimSpace(directory.ID))
	if err != nil {
		return nil, err
	}
	return personalAccountSummary(updated), nil
}

// UpdatePersonalConcurrency atomically validates the requested account limit
// against the user's current plan policy and persists it.
func (s *Service) UpdatePersonalConcurrency(ctx context.Context, ownerUserID string, maxConcurrentStreams int) (*PersonalAccountSummary, error) {
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID == "" {
		return nil, ErrOwnerUserIDRequired
	}
	if maxConcurrentStreams < 1 || maxConcurrentStreams > 100 {
		return nil, ErrMaxConcurrentStreamsInvalid
	}
	account, policy, err := s.store.UpdatePersonalConcurrency(ctx, ownerUserID, maxConcurrentStreams)
	if err != nil {
		return nil, err
	}
	summary := personalAccountSummary(account)
	if err := applyPersonalPlanPolicy(summary, policy); err != nil {
		return nil, err
	}
	return summary, nil
}

// SetPersonalEnabled atomically rechecks ownership, complete account state,
// and the current plan template before enabling. Disabling never needs a plan.
func (s *Service) SetPersonalEnabled(ctx context.Context, ownerUserID string, enabled bool) (*PersonalAccountSummary, error) {
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID == "" {
		return nil, ErrOwnerUserIDRequired
	}
	account, policy, err := s.store.SetPersonalEnabled(ctx, ownerUserID, enabled)
	if err != nil {
		return nil, err
	}
	summary := personalAccountSummary(account)
	if strings.TrimSpace(policy.PlanGroupKey) != "" {
		if err := applyPersonalPlanPolicy(summary, policy); err != nil {
			return nil, err
		}
	}
	return summary, nil
}

func effectivePersonalConcurrentLimit(configured, simultaneous int) (int, error) {
	if configured < 1 || configured > 100 || simultaneous < 0 || simultaneous > 100 {
		return 0, ErrMaxConcurrentStreamsInvalid
	}
	if simultaneous > 0 && configured > simultaneous {
		return 0, ErrMaxConcurrentStreamsExceedsPlan
	}
	return configured, nil
}

func applyPersonalPlanPolicy(summary *PersonalAccountSummary, policy PersonalPlanPolicy) error {
	if summary == nil || strings.TrimSpace(policy.PlanGroupKey) == "" ||
		(policy.PlaybackMode != models.P115PlaybackModePersonal && policy.PlaybackMode != models.P115PlaybackModeSystem) ||
		policy.TransferHourlyLimit < 1 || policy.TransferHourlyLimit > 100 ||
		policy.TransferDailyLimit < 1 || policy.TransferDailyLimit > 1000 ||
		policy.SimultaneousStreamLimit < 0 || policy.SimultaneousStreamLimit > 100 {
		return ErrPersonalPlanPolicyUnavailable
	}
	summary.PlaybackMode = policy.PlaybackMode
	summary.TransferHourlyLimit = intPointer(policy.TransferHourlyLimit)
	summary.TransferDailyLimit = intPointer(policy.TransferDailyLimit)
	summary.SimultaneousStreamLimit = intPointer(policy.SimultaneousStreamLimit)
	if summary.MaxConcurrentStreams != nil {
		effective := *summary.MaxConcurrentStreams
		if policy.SimultaneousStreamLimit > 0 && effective > policy.SimultaneousStreamLimit {
			effective = policy.SimultaneousStreamLimit
		}
		summary.EffectiveMaxConcurrentStreams = intPointer(effective)
	}
	return nil
}

func intPointer(value int) *int {
	return &value
}
