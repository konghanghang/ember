package p115

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"
)

const contractCheckRangeBytes int64 = 128 * 1024

// ReadOnlyContractProvider deliberately excludes every mutating Provider
// operation so the one-shot checker cannot initialize uploads or delete files.
type ReadOnlyContractProvider interface {
	ValidateCredential(ctx context.Context, credential Credential) (AccountIdentity, error)
	GetUploadInfo(ctx context.Context, credential Credential) (UploadInfo, error)
	ResolveFileByPath(ctx context.Context, credential Credential, query FilePathQuery) (*File, error)
	SearchBySHA1(ctx context.Context, credential Credential, query FileQuery) ([]File, error)
	GetDownloadURL(ctx context.Context, credential Credential, request DownloadURLRequest) (DownloadURLResult, error)
	HashFileRange(ctx context.Context, credential Credential, request FileRangeRequest) (FileRangeHash, error)
}

// ReadOnlyContractCheckInput contains terminal-supplied secrets and source
// identity used only in memory for one explicitly acknowledged real check.
type ReadOnlyContractCheckInput struct {
	SourceCredential    Credential
	PlaybackCredential  Credential
	SourceFile          FilePathQuery
	TestClientUserAgent string
}

// ContractCheckOutcome is the stable top-level result of a completed check.
type ContractCheckOutcome string

const (
	// ContractCheckPassed means every attempted read-only operation completed;
	// a playback SHA1 miss is reported separately and is not a protocol failure.
	ContractCheckPassed ContractCheckOutcome = "passed"
)

// ReadOnlyContractCheckReport is safe to print because it contains no
// credential, Provider identity, file path, pickCode, full hash, or signed URL.
type ReadOnlyContractCheckReport struct {
	SchemaVersion  int                         `json:"schemaVersion"`
	ReadOnly       bool                        `json:"readOnly"`
	Outcome        ContractCheckOutcome        `json:"outcome"`
	CompletedAt    time.Time                   `json:"completedAt"`
	Accounts       ContractCheckAccountsReport `json:"accounts"`
	SourceFile     ContractCheckSourceReport   `json:"sourceFile"`
	Playback       ContractCheckPlaybackReport `json:"playback"`
	SourceDownload ContractCheckDownloadReport `json:"sourceDownload"`
	Range          ContractCheckRangeReport    `json:"range"`
	Steps          []ContractCheckStepReport   `json:"steps"`
}

// ContractCheckAccountsReport exposes only validation relationships.
type ContractCheckAccountsReport struct {
	SourceValidated   bool `json:"sourceValidated"`
	PlaybackValidated bool `json:"playbackValidated"`
	UploadInfoMatched bool `json:"uploadInfoMatched"`
	Distinct          bool `json:"distinct"`
}

// ContractCheckSourceReport exposes only non-secret content evidence.
type ContractCheckSourceReport struct {
	Resolved   bool   `json:"resolved"`
	Size       int64  `json:"size"`
	SHA1Prefix string `json:"sha1Prefix"`
}

// ContractCheckPlaybackReport records whether the same content already exists
// and whether its final download URL contract could therefore be checked.
type ContractCheckPlaybackReport struct {
	Found             bool                         `json:"found"`
	DownloadValidated bool                         `json:"downloadValidated"`
	Download          *ContractCheckDownloadReport `json:"download,omitempty"`
}

// ContractCheckDownloadReport retains only non-reusable URL constraints.
type ContractCheckDownloadReport struct {
	Host                string             `json:"host,omitempty"`
	ExpiresAt           time.Time          `json:"expiresAt,omitempty"`
	HeaderMode          DownloadHeaderMode `json:"headerMode,omitempty"`
	ConcurrentOpenLimit int64              `json:"concurrentOpenLimit,omitempty"`
}

// ContractCheckRangeReport proves a bounded hash without returning source bytes.
type ContractCheckRangeReport struct {
	Start        int64  `json:"start"`
	End          int64  `json:"end"`
	BytesRead    int64  `json:"bytesRead"`
	HashComputed bool   `json:"hashComputed"`
	SHA1Prefix   string `json:"sha1Prefix"`
}

// ContractCheckStepReport records only fixed operation names and elapsed time.
type ContractCheckStepReport struct {
	Operation  string `json:"operation"`
	DurationMS int64  `json:"durationMs"`
}

type contractCheckError struct {
	stage string
	code  string
	err   error
}

// Error intentionally omits wrapped Provider text from terminal-facing paths.
func (err *contractCheckError) Error() string {
	return "p115 read-only contract check failed"
}

// Unwrap preserves errors.Is behavior for internal tests and callers.
func (err *contractCheckError) Unwrap() error {
	return err.err
}

// ContractCheckFailure converts an internal failure into fixed, non-secret
// stage and code values suitable for terminal output.
func ContractCheckFailure(err error) (stage, code string) {
	var contractErr *contractCheckError
	if errors.As(err, &contractErr) {
		return contractErr.stage, contractErr.code
	}
	if err == nil {
		return "", ""
	}
	return "unknown", "unknown"
}

// RunReadOnlyContractCheck executes the fixed metadata, source-resolution,
// download-contract, and 128 KiB Range sequence without any write operation.
func RunReadOnlyContractCheck(
	ctx context.Context,
	provider ReadOnlyContractProvider,
	input ReadOnlyContractCheckInput,
) (*ReadOnlyContractCheckReport, error) {
	if err := validateReadOnlyContractCheckInput(provider, input); err != nil {
		return nil, contractFailure("input", "invalid_input", err)
	}
	report := &ReadOnlyContractCheckReport{
		SchemaVersion: 1,
		ReadOnly:      true,
		Steps:         make([]ContractCheckStepReport, 0, 9),
	}

	started := time.Now()
	sourceIdentity, err := provider.ValidateCredential(ctx, input.SourceCredential)
	if err != nil {
		return nil, providerContractFailure("source_credential", err)
	}
	recordContractStep(report, "validate:source", started)

	started = time.Now()
	playbackIdentity, err := provider.ValidateCredential(ctx, input.PlaybackCredential)
	if err != nil {
		return nil, providerContractFailure("playback_credential", err)
	}
	recordContractStep(report, "validate:playback", started)

	if strings.TrimSpace(sourceIdentity.ProviderUserID) == "" || strings.TrimSpace(playbackIdentity.ProviderUserID) == "" {
		return nil, contractFailure("account_identity", "identity_missing", ErrProviderProtocol)
	}
	if sourceIdentity.ProviderUserID == playbackIdentity.ProviderUserID {
		return nil, contractFailure("account_identity", "accounts_same", ErrCredentialRejected)
	}
	report.Accounts.SourceValidated = true
	report.Accounts.PlaybackValidated = true
	report.Accounts.Distinct = true

	started = time.Now()
	sourceUploadInfo, err := provider.GetUploadInfo(ctx, input.SourceCredential)
	if err != nil {
		return nil, providerContractFailure("source_upload_info", err)
	}
	recordContractStep(report, "upload_info:source", started)
	if sourceUploadInfo.UserID != sourceIdentity.ProviderUserID {
		return nil, contractFailure("source_upload_info", "identity_mismatch", ErrCredentialRejected)
	}

	started = time.Now()
	playbackUploadInfo, err := provider.GetUploadInfo(ctx, input.PlaybackCredential)
	if err != nil {
		return nil, providerContractFailure("playback_upload_info", err)
	}
	recordContractStep(report, "upload_info:playback", started)
	if playbackUploadInfo.UserID != playbackIdentity.ProviderUserID {
		return nil, contractFailure("playback_upload_info", "identity_mismatch", ErrCredentialRejected)
	}
	report.Accounts.UploadInfoMatched = true

	started = time.Now()
	sourceFile, err := provider.ResolveFileByPath(ctx, input.SourceCredential, input.SourceFile)
	if err != nil {
		return nil, providerContractFailure("source_resolve", err)
	}
	recordContractStep(report, "resolve_source", started)
	if sourceFile == nil || sourceFile.IsDirectory || sourceFile.Size != input.SourceFile.Size ||
		strings.TrimSpace(sourceFile.PickCode) == "" {
		return nil, contractFailure("source_resolve", "response_invalid", ErrProviderProtocol)
	}
	sourceSHA1, err := normalizeSHA1(sourceFile.SHA1)
	if err != nil {
		return nil, contractFailure("source_resolve", "response_invalid", ErrProviderProtocol)
	}
	report.SourceFile = ContractCheckSourceReport{
		Resolved: true, Size: sourceFile.Size, SHA1Prefix: hashPrefix(sourceSHA1),
	}

	started = time.Now()
	playbackFiles, err := provider.SearchBySHA1(ctx, input.PlaybackCredential, FileQuery{
		SHA1: sourceSHA1, Size: sourceFile.Size,
	})
	if err != nil {
		return nil, providerContractFailure("playback_search", err)
	}
	recordContractStep(report, "search_playback", started)
	if len(playbackFiles) > 1 {
		return nil, contractFailure("playback_search", "response_invalid", ErrProviderProtocol)
	}
	report.Playback.Found = len(playbackFiles) == 1

	started = time.Now()
	sourceDownload, err := provider.GetDownloadURL(ctx, input.SourceCredential, DownloadURLRequest{
		PickCode: sourceFile.PickCode, UserAgent: input.SourceCredential.UserAgent,
	})
	if err != nil {
		return nil, providerContractFailure("source_download", err)
	}
	recordContractStep(report, "download:source", started)
	report.SourceDownload, err = contractDownloadReport(sourceDownload)
	if err != nil {
		return nil, contractFailure("source_download", "response_invalid", err)
	}

	rangeEnd := contractCheckRangeBytes - 1
	if sourceFile.Size <= contractCheckRangeBytes {
		rangeEnd = sourceFile.Size - 1
	}
	rangeRequest := FileRangeRequest{
		File: *sourceFile, Range: ByteRange{Start: 0, End: rangeEnd},
	}
	started = time.Now()
	rangeHash, err := provider.HashFileRange(ctx, input.SourceCredential, rangeRequest)
	if err != nil {
		return nil, providerContractFailure("source_range", err)
	}
	recordContractStep(report, "hash_range", started)
	expectedBytes := rangeEnd + 1
	normalizedRangeSHA1, normalizeErr := normalizeSHA1(rangeHash.SHA1)
	if normalizeErr != nil || rangeHash.BytesRead != expectedBytes {
		return nil, contractFailure("source_range", "response_invalid", ErrProviderProtocol)
	}
	report.Range = ContractCheckRangeReport{
		Start: 0, End: rangeEnd, BytesRead: rangeHash.BytesRead,
		HashComputed: true, SHA1Prefix: hashPrefix(normalizedRangeSHA1),
	}

	if len(playbackFiles) == 1 {
		candidate := playbackFiles[0]
		if candidate.IsDirectory || candidate.SHA1 != sourceSHA1 || candidate.Size != sourceFile.Size ||
			strings.TrimSpace(candidate.PickCode) == "" {
			return nil, contractFailure("playback_search", "response_invalid", ErrProviderProtocol)
		}
		started = time.Now()
		playbackDownload, err := provider.GetDownloadURL(ctx, input.PlaybackCredential, DownloadURLRequest{
			PickCode: candidate.PickCode, UserAgent: input.TestClientUserAgent,
		})
		if err != nil {
			return nil, providerContractFailure("playback_download", err)
		}
		recordContractStep(report, "download:playback", started)
		playbackDownloadReport, err := contractDownloadReport(playbackDownload)
		if err != nil {
			return nil, contractFailure("playback_download", "response_invalid", err)
		}
		report.Playback.Download = &playbackDownloadReport
		report.Playback.DownloadValidated = true
	}

	report.Outcome = ContractCheckPassed
	report.CompletedAt = time.Now().UTC()
	return report, nil
}

func validateReadOnlyContractCheckInput(provider ReadOnlyContractProvider, input ReadOnlyContractCheckInput) error {
	if provider == nil || strings.TrimSpace(input.SourceCredential.AccountID) == "" ||
		strings.TrimSpace(input.PlaybackCredential.AccountID) == "" ||
		strings.TrimSpace(input.SourceCredential.Cookie) == "" ||
		strings.TrimSpace(input.PlaybackCredential.Cookie) == "" ||
		input.SourceCredential.Cookie == input.PlaybackCredential.Cookie ||
		strings.TrimSpace(input.SourceCredential.UserAgent) == "" ||
		strings.TrimSpace(input.PlaybackCredential.UserAgent) == "" ||
		strings.TrimSpace(input.TestClientUserAgent) == "" || input.SourceFile.Size <= 0 ||
		strings.TrimSpace(input.SourceFile.RootID) == "" || strings.TrimSpace(input.SourceFile.RelativePath) == "" {
		return ErrInvalidRequest
	}
	if strings.ContainsAny(input.SourceCredential.Cookie+input.PlaybackCredential.Cookie+
		input.SourceCredential.UserAgent+input.PlaybackCredential.UserAgent+input.TestClientUserAgent, "\r\n") {
		return ErrInvalidRequest
	}
	return nil
}

func contractDownloadReport(result DownloadURLResult) (ContractCheckDownloadReport, error) {
	parsed, err := url.Parse(result.URL)
	if err != nil || parsed.Hostname() == "" || result.ExpiresAt.IsZero() {
		return ContractCheckDownloadReport{}, ErrProviderProtocol
	}
	return ContractCheckDownloadReport{
		Host: parsed.Hostname(), ExpiresAt: result.ExpiresAt.UTC(), HeaderMode: result.HeaderMode,
		ConcurrentOpenLimit: result.ConcurrentOpenLimit,
	}, nil
}

func recordContractStep(report *ReadOnlyContractCheckReport, operation string, started time.Time) {
	report.Steps = append(report.Steps, ContractCheckStepReport{
		Operation: operation, DurationMS: time.Since(started).Milliseconds(),
	})
}

func hashPrefix(value string) string {
	if len(value) <= 8 {
		return value
	}
	return value[:8]
}

func contractFailure(stage, code string, err error) error {
	return &contractCheckError{stage: stage, code: code, err: err}
}

func providerContractFailure(stage string, err error) error {
	return contractFailure(stage, providerContractErrorCode(err), err)
}

func providerContractErrorCode(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case errors.Is(err, ErrCredentialRejected):
		return "credential_rejected"
	case errors.Is(err, ErrDownloadURLExpired):
		return "download_expired"
	case errors.Is(err, ErrDownloadURLIncompatible):
		return "download_incompatible"
	case errors.Is(err, ErrDownloadURLNotAllowed):
		return "download_host_not_allowed"
	case errors.Is(err, ErrDirectoryAmbiguous):
		return "directory_ambiguous"
	case errors.Is(err, ErrDirectoryNotFound):
		return "directory_not_found"
	case errors.Is(err, ErrInvalidRequest):
		return "invalid_request"
	case errors.Is(err, ErrProviderRejected):
		return "provider_rejected"
	case errors.Is(err, ErrProviderUnavailable):
		return "provider_unavailable"
	case errors.Is(err, ErrProviderProtocol):
		return "provider_protocol"
	case errors.Is(err, ErrSourceDirectoryTooLarge):
		return "source_directory_too_large"
	case errors.Is(err, ErrSourceFileAmbiguous):
		return "source_file_ambiguous"
	case errors.Is(err, ErrSourceFileNotFound):
		return "source_file_not_found"
	case errors.Is(err, ErrTargetFileAmbiguous):
		return "target_file_ambiguous"
	case errors.Is(err, ErrTargetFileNotVisible):
		return "target_file_not_visible"
	default:
		return "unknown"
	}
}

var _ ReadOnlyContractProvider = (*CookieProvider)(nil)
