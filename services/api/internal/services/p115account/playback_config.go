package p115account

import (
	"context"
	"math"
	"strings"

	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/models"
)

const p115RootDirectoryID = "0"

// PlaybackConfigInput atomically replaces an administrator playback account's
// resolved target directory and account-wide concurrent stream limit.
type PlaybackConfigInput struct {
	TargetParentPath     string `json:"targetParentPath"`
	MaxConcurrentStreams int    `json:"maxConcurrentStreams"`
}

// UpdatePlaybackConfig resolves one existing 115 directory and persists its
// path, ID, and concurrency value only if the credential generation is unchanged.
func (s *Service) UpdatePlaybackConfig(ctx context.Context, accountID string, input PlaybackConfigInput) (*AccountSummary, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, ErrAccountIDRequired
	}
	input.TargetParentPath = strings.TrimSpace(input.TargetParentPath)
	if input.TargetParentPath == "" {
		return nil, ErrTargetParentPathRequired
	}
	if input.MaxConcurrentStreams < 1 || int64(input.MaxConcurrentStreams) > math.MaxInt32 {
		return nil, ErrMaxConcurrentStreamsInvalid
	}
	if s.directoryResolver == nil {
		return nil, ErrDirectoryResolverUnavailable
	}

	account, err := s.getAdminAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account.Role != models.P115AccountRolePlayback {
		return nil, ErrPlaybackConfigOnly
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
	}, p115integration.DirectoryPathQuery{
		RootID:       p115RootDirectoryID,
		RelativePath: input.TargetParentPath,
	})
	if err != nil {
		return nil, err
	}
	if directory == nil || strings.TrimSpace(directory.ID) == "" || strings.TrimSpace(directory.Path) == "" {
		return nil, p115integration.ErrProviderProtocol
	}

	updated, err := s.store.UpdatePlaybackConfig(
		ctx,
		account.ID,
		ciphertext,
		account.UpdatedAt,
		strings.TrimSpace(directory.Path),
		strings.TrimSpace(directory.ID),
		input.MaxConcurrentStreams,
	)
	if err != nil {
		return nil, err
	}
	return accountSummary(updated), nil
}
