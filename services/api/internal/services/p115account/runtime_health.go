package p115account

import (
	"context"
	"log"
	"time"

	"github.com/konghang/ember/backend/internal/models"
)

const runtimeProviderCooldown = time.Minute

// RuntimeHealthOutcome is the bounded account-wide result that DirectPlay may
// report. Provider errors and response bodies never cross this boundary.
type RuntimeHealthOutcome string

const (
	RuntimeHealthSucceeded           RuntimeHealthOutcome = "succeeded"
	RuntimeHealthCredentialRejected  RuntimeHealthOutcome = "credential_rejected"
	RuntimeHealthProviderUnavailable RuntimeHealthOutcome = "provider_unavailable"
	RuntimeHealthProviderProtocol    RuntimeHealthOutcome = "provider_protocol"
)

type runtimeCredentialRef struct {
	accountID          string
	expectedCiphertext string
	expectedUpdatedAt  time.Time
}

type runtimeHealthMutation struct {
	Status        models.P115AccountStatus
	Disable       bool
	Succeeded     bool
	CooldownUntil *time.Time
	Code          string
	Message       string
	At            time.Time
}

// ReportRuntimeHealth persists one account-wide playback result only when the
// same credential generation is still current. Stale requests cannot override
// a Cookie replacement, validation, manual toggle, or newer runtime outcome.
func (s *Service) ReportRuntimeHealth(
	ctx context.Context,
	account ActiveAccountCredential,
	outcome RuntimeHealthOutcome,
) error {
	ref := account.runtimeRef
	if ref.accountID == "" || ref.expectedCiphertext == "" || ref.expectedUpdatedAt.IsZero() {
		return ErrRuntimeStateChanged
	}
	now := s.now().UTC()
	mutation, err := buildRuntimeHealthMutation(outcome, now)
	if err != nil {
		return err
	}
	if err := s.store.CompleteRuntimeHealth(ctx, ref, mutation); err != nil {
		return err
	}
	log.Printf("[P115Account] 运行期健康已更新 accountId=%s role=%s outcome=%s status=%s",
		ref.accountID, account.Role, outcome, mutation.Status)
	return nil
}

// buildRuntimeHealthMutation maps the bounded outcome to one fixed persisted
// state without accepting Provider text or caller-supplied error codes.
func buildRuntimeHealthMutation(outcome RuntimeHealthOutcome, now time.Time) (runtimeHealthMutation, error) {
	mutation := runtimeHealthMutation{At: now}
	switch outcome {
	case RuntimeHealthSucceeded:
		mutation.Status = models.P115AccountStatusActive
		mutation.Succeeded = true
	case RuntimeHealthCredentialRejected:
		mutation.Status = models.P115AccountStatusExpired
		mutation.Disable = true
		mutation.Code = validationCodeRejected
		mutation.Message = "115 Cookie 已失效"
	case RuntimeHealthProviderUnavailable:
		mutation.Status = models.P115AccountStatusCoolingDown
		cooldownUntil := now.Add(runtimeProviderCooldown)
		mutation.CooldownUntil = &cooldownUntil
		mutation.Code = validationCodeUnavailable
		mutation.Message = "115 服务暂不可用"
	case RuntimeHealthProviderProtocol:
		mutation.Status = models.P115AccountStatusError
		mutation.Code = validationCodeProtocol
		mutation.Message = "115 响应格式不兼容"
	default:
		return runtimeHealthMutation{}, ErrRuntimeHealthOutcomeInvalid
	}
	return mutation, nil
}
