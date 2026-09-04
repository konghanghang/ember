package p115account

import (
	"context"
	"errors"
	"strings"
	"time"

	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/models"
)

// PlaybackRoute is the non-secret account selection used before Redis
// admission. Cookie ciphertext is deliberately absent from this boundary.
type PlaybackRoute struct {
	PlaybackMode                   models.P115PlaybackMode
	AccountID                      string
	OwnerUserID                    string
	ProviderUserID                 string
	TargetParentID                 string
	TargetParentPath               string
	ConfiguredMaxConcurrentStreams int
	EffectiveMaxConcurrentStreams  int
	SimultaneousStreamLimit        int
	TransferHourlyLimit            int
	TransferDailyLimit             int
	Status                         models.P115AccountStatus
	CooldownUntil                  *time.Time
	UpdatedAt                      time.Time
}

// ResolvePlaybackRoute reads the current plan and one exact playback account
// without loading or decrypting credential fields.
func (s *Service) ResolvePlaybackRoute(ctx context.Context, ownerUserID string, now time.Time) (PlaybackRoute, error) {
	ownerUserID = strings.TrimSpace(ownerUserID)
	if ownerUserID == "" {
		return PlaybackRoute{}, ErrOwnerUserIDRequired
	}
	account, policy, err := s.store.ResolvePlaybackRouteMetadata(ctx, ownerUserID)
	route := PlaybackRoute{
		PlaybackMode: policy.PlaybackMode, SimultaneousStreamLimit: policy.SimultaneousStreamLimit,
		TransferHourlyLimit: policy.TransferHourlyLimit, TransferDailyLimit: policy.TransferDailyLimit,
	}
	if err != nil {
		if policy.PlaybackMode == models.P115PlaybackModePersonal && errors.Is(err, ErrAccountNotFound) {
			return route, ErrPersonalAccountMissing
		}
		return route, err
	}
	if err := validatePlaybackRouteAccount(account, policy.PlaybackMode, ownerUserID, now); err != nil {
		return route, err
	}

	configured := *account.MaxConcurrentStreams
	effective := configured
	if policy.PlaybackMode == models.P115PlaybackModePersonal && policy.SimultaneousStreamLimit > 0 && effective > policy.SimultaneousStreamLimit {
		effective = policy.SimultaneousStreamLimit
	}
	owner := ""
	if account.OwnerUserID != nil {
		owner = *account.OwnerUserID
	}
	return PlaybackRoute{
		PlaybackMode: policy.PlaybackMode, AccountID: account.ID, OwnerUserID: owner,
		ProviderUserID: strings.TrimSpace(*account.ProviderUserID), TargetParentID: strings.TrimSpace(*account.TargetParentID),
		TargetParentPath: strings.TrimSpace(*account.TargetParentPath), ConfiguredMaxConcurrentStreams: configured,
		EffectiveMaxConcurrentStreams: effective, SimultaneousStreamLimit: policy.SimultaneousStreamLimit,
		TransferHourlyLimit: policy.TransferHourlyLimit, TransferDailyLimit: policy.TransferDailyLimit,
		Status: account.Status, CooldownUntil: account.CooldownUntil, UpdatedAt: account.UpdatedAt,
	}, nil
}

// AcquirePlaybackRoute locks and loads the exact account generation selected
// before Redis admission, including at most one expired-cooldown probe lease.
func (s *Service) AcquirePlaybackRoute(ctx context.Context, route PlaybackRoute) (ActiveAccountCredential, error) {
	now := s.now().UTC()
	account, err := s.store.AcquirePlaybackRoute(ctx, route, now, now.Add(runtimeProviderCooldown))
	if err != nil {
		return ActiveAccountCredential{}, err
	}
	if err := validateAcquiredPlaybackRoute(account, route); err != nil {
		return ActiveAccountCredential{}, err
	}
	ciphertext, appType, userAgent, err := requiredCredentialFields(account)
	if err != nil {
		return ActiveAccountCredential{}, err
	}
	cookie, err := s.cipher.Decrypt(ciphertext)
	if err != nil {
		return ActiveAccountCredential{}, err
	}
	return ActiveAccountCredential{
		Role: models.P115AccountRolePlayback, ProviderUserID: route.ProviderUserID, TargetParentID: route.TargetParentID,
		Credential: p115integration.Credential{AccountID: account.ID, Cookie: cookie, AppType: appType, UserAgent: userAgent},
		runtimeRef: runtimeCredentialRef{accountID: account.ID, expectedCiphertext: ciphertext, expectedUpdatedAt: account.UpdatedAt},
	}, nil
}

func validatePlaybackRouteAccount(account *models.P115Account, mode models.P115PlaybackMode, ownerUserID string, now time.Time) error {
	if account == nil || account.Role != models.P115AccountRolePlayback || !account.Enabled ||
		account.ProviderUserID == nil || strings.TrimSpace(*account.ProviderUserID) == "" ||
		account.TargetParentID == nil || strings.TrimSpace(*account.TargetParentID) == "" ||
		account.TargetParentPath == nil || strings.TrimSpace(*account.TargetParentPath) == "" ||
		account.MaxConcurrentStreams == nil || *account.MaxConcurrentStreams < 1 {
		return ErrAccountUnavailable
	}
	if mode == models.P115PlaybackModePersonal {
		if account.OwnerUserID == nil || *account.OwnerUserID != ownerUserID {
			return ErrAccountUnavailable
		}
	} else if mode == models.P115PlaybackModeSystem {
		if account.OwnerUserID != nil {
			return ErrAccountUnavailable
		}
	} else {
		return ErrPersonalPlanPolicyUnavailable
	}
	switch account.Status {
	case models.P115AccountStatusActive:
		return nil
	case models.P115AccountStatusCoolingDown:
		if account.CooldownUntil != nil && !account.CooldownUntil.After(now) {
			return nil
		}
		return ErrAccountCoolingDown
	default:
		return ErrAccountUnavailable
	}
}

func validateAcquiredPlaybackRoute(account *models.P115Account, route PlaybackRoute) error {
	if account == nil || account.ID != route.AccountID || account.Role != models.P115AccountRolePlayback || !account.Enabled ||
		account.ProviderUserID == nil || strings.TrimSpace(*account.ProviderUserID) != route.ProviderUserID ||
		account.TargetParentID == nil || strings.TrimSpace(*account.TargetParentID) != route.TargetParentID ||
		account.TargetParentPath == nil || strings.TrimSpace(*account.TargetParentPath) != route.TargetParentPath ||
		account.MaxConcurrentStreams == nil || *account.MaxConcurrentStreams != route.ConfiguredMaxConcurrentStreams {
		return ErrRuntimeStateChanged
	}
	if route.OwnerUserID == "" {
		if account.OwnerUserID != nil {
			return ErrRuntimeStateChanged
		}
	} else if account.OwnerUserID == nil || *account.OwnerUserID != route.OwnerUserID {
		return ErrRuntimeStateChanged
	}
	if account.Status != models.P115AccountStatusActive && account.Status != models.P115AccountStatusCoolingDown {
		return ErrRuntimeStateChanged
	}
	return nil
}
