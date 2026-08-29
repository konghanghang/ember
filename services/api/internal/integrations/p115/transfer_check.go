package p115

import (
	"context"
	"errors"
	"strings"
	"time"
)

// TransferContractProvider contains every operation required to prove one
// retained playback transfer and deliberately excludes DeleteFile.
type TransferContractProvider interface {
	ValidateCredential(ctx context.Context, credential Credential) (AccountIdentity, error)
	GetUploadInfo(ctx context.Context, credential Credential) (UploadInfo, error)
	ResolveFileByPath(ctx context.Context, credential Credential, query FilePathQuery) (*File, error)
	ResolveDirectoryByPath(ctx context.Context, credential Credential, query DirectoryPathQuery) (*Directory, error)
	SearchBySHA1(ctx context.Context, credential Credential, query FileQuery) ([]File, error)
	InitRapidUpload(ctx context.Context, credential Credential, request RapidUploadRequest) (RapidUploadResult, error)
	FindTargetFile(ctx context.Context, credential Credential, query FileQuery) (*File, error)
	GetDownloadURL(ctx context.Context, credential Credential, request DownloadURLRequest) (DownloadURLResult, error)
	HashFileRange(ctx context.Context, credential Credential, request FileRangeRequest) (FileRangeHash, error)
}

// TransferContractCheckInput contains one source file and one playback target
// directory used only in memory for an explicitly acknowledged retained write.
type TransferContractCheckInput struct {
	SourceCredential    Credential
	PlaybackCredential  Credential
	SourceFile          FilePathQuery
	ExpectedSourceSize  int64
	TargetDirectory     DirectoryPathQuery
	TestClientUserAgent string
}

// TransferContractCheckReport is safe to print and intentionally omits every
// credential, file/directory ID, path, pickCode, full hash, and signed URL.
type TransferContractCheckReport struct {
	SchemaVersion    int                           `json:"schemaVersion"`
	WriteCapable     bool                          `json:"writeCapable"`
	WritePerformed   bool                          `json:"writePerformed"`
	Outcome          ContractCheckOutcome          `json:"outcome"`
	CompletedAt      time.Time                     `json:"completedAt"`
	Accounts         ContractCheckAccountsReport   `json:"accounts"`
	SourceFile       ContractCheckSourceReport     `json:"sourceFile"`
	TargetDirectory  TransferTargetDirectoryReport `json:"targetDirectory"`
	Transfer         TransferResultReport          `json:"transfer"`
	PlaybackDownload ContractCheckDownloadReport   `json:"playbackDownload"`
	PlaybackRange    ContractCheckRangeReport      `json:"playbackRange"`
	Cleanup          TransferCleanupReport         `json:"cleanup"`
	Steps            []ContractCheckStepReport     `json:"steps"`
}

// TransferTargetDirectoryReport exposes only whether unique resolution passed.
type TransferTargetDirectoryReport struct {
	Resolved bool `json:"resolved"`
}

// TransferResultReport describes write semantics without reusable file identity.
type TransferResultReport struct {
	Preexisting           bool              `json:"preexisting"`
	Created               bool              `json:"created"`
	Retained              bool              `json:"retained"`
	SecondCheckPerformed  bool              `json:"secondCheckPerformed"`
	DatabaseLockValidated bool              `json:"databaseLockValidated"`
	InitialStatus         RapidUploadStatus `json:"initialStatus,omitempty"`
	FinalStatus           RapidUploadStatus `json:"finalStatus,omitempty"`
	ChallengeCount        int               `json:"challengeCount"`
	TargetVisibleTimeMS   int64             `json:"targetVisibleTimeMs,omitempty"`
}

// TransferCleanupReport makes the phase-one no-delete decision explicit.
type TransferCleanupReport struct {
	Attempted bool `json:"attempted"`
}

type transferContractCheckError struct {
	stage        string
	code         string
	fileMayExist bool
	err          error
}

// Error intentionally omits Provider and retained file details.
func (err *transferContractCheckError) Error() string {
	return "p115 retained transfer contract check failed"
}

// Unwrap preserves errors.Is behavior for diagnostics and tests.
func (err *transferContractCheckError) Unwrap() error {
	return err.err
}

// TransferContractCheckFailure returns fixed terminal-safe failure metadata.
func TransferContractCheckFailure(err error) (stage, code string, fileMayExist bool) {
	var transferErr *transferContractCheckError
	if errors.As(err, &transferErr) {
		return transferErr.stage, transferErr.code, transferErr.fileMayExist
	}
	if err == nil {
		return "", "", false
	}
	return "unknown", "unknown", false
}

// RunTransferContractCheck proves a single retained playback transfer or
// preexisting-file fast path without ever exposing a delete capability.
func RunTransferContractCheck(
	ctx context.Context,
	provider TransferContractProvider,
	input TransferContractCheckInput,
) (*TransferContractCheckReport, error) {
	if err := validateTransferContractInput(provider, input); err != nil {
		return nil, transferFailure("input", "invalid_input", false, err)
	}
	report := &TransferContractCheckReport{
		SchemaVersion: 1,
		WriteCapable:  true,
		Cleanup:       TransferCleanupReport{Attempted: false},
		Steps:         make([]ContractCheckStepReport, 0, 16),
	}

	sourceIdentity, err := transferValidateCredential(ctx, provider, input.SourceCredential, report)
	if err != nil {
		return nil, err
	}
	playbackIdentity, err := transferValidateCredential(ctx, provider, input.PlaybackCredential, report)
	if err != nil {
		return nil, err
	}
	if sourceIdentity.ProviderUserID == playbackIdentity.ProviderUserID {
		return nil, transferFailure("account_identity", "accounts_same", false, ErrCredentialRejected)
	}
	report.Accounts.SourceValidated = true
	report.Accounts.PlaybackValidated = true
	report.Accounts.Distinct = true

	if err := transferValidateUploadInfo(ctx, provider, input.SourceCredential, sourceIdentity, report); err != nil {
		return nil, err
	}
	if err := transferValidateUploadInfo(ctx, provider, input.PlaybackCredential, playbackIdentity, report); err != nil {
		return nil, err
	}
	report.Accounts.UploadInfoMatched = true

	started := time.Now()
	sourceFile, err := provider.ResolveFileByPath(ctx, input.SourceCredential, input.SourceFile)
	if err != nil {
		return nil, transferProviderFailure("source_resolve", false, err)
	}
	recordTransferStep(report, "resolve_source", started)
	if !validTransferFile(sourceFile, input.ExpectedSourceSize, "") {
		return nil, transferFailure("source_resolve", "response_invalid", false, ErrProviderProtocol)
	}
	sourceSHA1, err := normalizeSHA1(sourceFile.SHA1)
	if err != nil {
		return nil, transferFailure("source_resolve", "response_invalid", false, ErrProviderProtocol)
	}
	report.SourceFile = ContractCheckSourceReport{Resolved: true, Size: sourceFile.Size, SHA1Prefix: hashPrefix(sourceSHA1)}

	started = time.Now()
	targetDirectory, err := provider.ResolveDirectoryByPath(ctx, input.PlaybackCredential, input.TargetDirectory)
	if err != nil {
		return nil, transferProviderFailure("target_directory", false, err)
	}
	recordTransferStep(report, "resolve_directory", started)
	if targetDirectory == nil || targetDirectory.ID == "" || targetDirectory.ID == "0" {
		return nil, transferFailure("target_directory", "response_invalid", false, ErrProviderProtocol)
	}
	report.TargetDirectory.Resolved = true

	query := FileQuery{SHA1: sourceSHA1, Size: sourceFile.Size, ParentID: targetDirectory.ID}
	targetFile, preexisting, err := transferSearchPlayback(ctx, provider, input.PlaybackCredential, query, report)
	if err != nil {
		return nil, err
	}
	fileMayExist := preexisting

	if !preexisting {
		report.Transfer.SecondCheckPerformed = true
		targetFile, preexisting, err = transferSearchPlayback(ctx, provider, input.PlaybackCredential, query, report)
		if err != nil {
			return nil, err
		}
		fileMayExist = preexisting
	}

	if !preexisting {
		preIDRange := boundedContractRange(sourceFile.Size)
		started = time.Now()
		preIDHash, err := provider.HashFileRange(ctx, input.SourceCredential, FileRangeRequest{
			File: *sourceFile, Range: preIDRange,
		})
		if err != nil {
			return nil, transferProviderFailure("source_preid", false, err)
		}
		recordTransferStep(report, "hash_preid", started)
		preID, err := validateTransferRangeHash(preIDHash, preIDRange)
		if err != nil {
			return nil, transferFailure("source_preid", "response_invalid", false, err)
		}

		uploadRequest := RapidUploadRequest{
			FileName: sourceFile.Name, SHA1: sourceSHA1, Size: sourceFile.Size,
			TargetParentID: targetDirectory.ID, PreID: preID,
		}
		started = time.Now()
		result, err := provider.InitRapidUpload(ctx, input.PlaybackCredential, uploadRequest)
		if err != nil {
			return nil, transferProviderFailure("rapid_upload", false, err)
		}
		recordTransferStep(report, "init_upload", started)
		report.Transfer.InitialStatus = result.Status

		if result.Status == RapidUploadRangeChallenge {
			if result.Challenge == nil {
				return nil, transferFailure("rapid_upload", "response_invalid", false, ErrProviderProtocol)
			}
			report.Transfer.ChallengeCount = 1
			started = time.Now()
			challengeHash, err := provider.HashFileRange(ctx, input.SourceCredential, FileRangeRequest{
				File: *sourceFile, Range: result.Challenge.Range,
			})
			if err != nil {
				return nil, transferProviderFailure("source_challenge", false, err)
			}
			recordTransferStep(report, "hash_challenge", started)
			signValue, err := validateTransferRangeHash(challengeHash, result.Challenge.Range)
			if err != nil {
				return nil, transferFailure("source_challenge", "response_invalid", false, err)
			}
			uploadRequest.SignKey = result.Challenge.SignKey
			uploadRequest.SignValue = signValue
			started = time.Now()
			result, err = provider.InitRapidUpload(ctx, input.PlaybackCredential, uploadRequest)
			if err != nil {
				return nil, transferProviderFailure("rapid_upload", false, err)
			}
			recordTransferStep(report, "init_upload_retry", started)
			if result.Status == RapidUploadRangeChallenge {
				return nil, transferFailure("rapid_upload", "repeated_challenge", false, ErrProviderProtocol)
			}
		}

		switch result.Status {
		case RapidUploadReused:
			fileMayExist = true
			report.Transfer.Created = true
			report.Transfer.FinalStatus = RapidUploadReused
		case RapidUploadOrdinaryUploadRequired:
			return nil, transferFailure("rapid_upload", "ordinary_upload_required", false, ErrProviderRejected)
		default:
			return nil, transferFailure("rapid_upload", "provider_rejected", false, ErrProviderRejected)
		}

		visibleStarted := time.Now()
		targetFile, err = provider.FindTargetFile(ctx, input.PlaybackCredential, query)
		if err != nil {
			return nil, transferProviderFailure("target_verify", fileMayExist, err)
		}
		report.Transfer.TargetVisibleTimeMS = time.Since(visibleStarted).Milliseconds()
		recordTransferStep(report, "find_target", visibleStarted)
	}

	if !validTransferFile(targetFile, sourceFile.Size, targetDirectory.ID) || targetFile.SHA1 != sourceSHA1 {
		return nil, transferFailure("target_verify", "response_invalid", fileMayExist, ErrProviderProtocol)
	}
	report.Transfer.Preexisting = preexisting
	report.Transfer.Retained = true
	report.WritePerformed = report.Transfer.Created

	started = time.Now()
	playbackDownload, err := provider.GetDownloadURL(ctx, input.PlaybackCredential, DownloadURLRequest{
		PickCode: targetFile.PickCode, UserAgent: input.TestClientUserAgent,
	})
	if err != nil {
		return nil, transferProviderFailure("playback_download", true, err)
	}
	recordTransferStep(report, "download_playback", started)
	if playbackDownload.HeaderMode == DownloadHeadersSameUserAgentAndCookie {
		return nil, transferFailure("playback_download", "download_incompatible", true, ErrDownloadURLIncompatible)
	}
	report.PlaybackDownload, err = contractDownloadReport(playbackDownload)
	if err != nil {
		return nil, transferFailure("playback_download", "response_invalid", true, err)
	}

	playbackRange := boundedContractRange(targetFile.Size)
	started = time.Now()
	playbackHash, err := provider.HashFileRange(ctx, input.PlaybackCredential, FileRangeRequest{
		File: *targetFile, Range: playbackRange, UserAgent: input.TestClientUserAgent,
	})
	if err != nil {
		return nil, transferProviderFailure("playback_range", true, err)
	}
	recordTransferStep(report, "hash_playback_range", started)
	playbackSHA1, err := validateTransferRangeHash(playbackHash, playbackRange)
	if err != nil {
		return nil, transferFailure("playback_range", "response_invalid", true, err)
	}
	report.PlaybackRange = ContractCheckRangeReport{
		Start: playbackRange.Start, End: playbackRange.End, BytesRead: playbackHash.BytesRead,
		HashComputed: true, SHA1Prefix: hashPrefix(playbackSHA1),
	}
	report.Outcome = ContractCheckPassed
	report.CompletedAt = time.Now().UTC()
	return report, nil
}

func validateTransferContractInput(provider TransferContractProvider, input TransferContractCheckInput) error {
	if provider == nil || input.ExpectedSourceSize <= 0 || strings.TrimSpace(input.TestClientUserAgent) == "" ||
		strings.ContainsAny(input.TestClientUserAgent, "\r\n") ||
		strings.TrimSpace(input.SourceCredential.Cookie) == "" || strings.TrimSpace(input.PlaybackCredential.Cookie) == "" ||
		input.SourceCredential.Cookie == input.PlaybackCredential.Cookie {
		return ErrInvalidRequest
	}
	if _, err := normalizeFilePathQuery(input.SourceFile); err != nil {
		return err
	}
	if _, err := normalizeDirectoryPathQuery(input.TargetDirectory); err != nil {
		return err
	}
	return nil
}

func transferValidateCredential(
	ctx context.Context,
	provider TransferContractProvider,
	credential Credential,
	report *TransferContractCheckReport,
) (AccountIdentity, error) {
	started := time.Now()
	identity, err := provider.ValidateCredential(ctx, credential)
	if err != nil {
		return AccountIdentity{}, transferProviderFailure(credential.AccountID+"_credential", false, err)
	}
	recordTransferStep(report, "validate:"+credential.AccountID, started)
	if strings.TrimSpace(identity.ProviderUserID) == "" {
		return AccountIdentity{}, transferFailure("account_identity", "identity_missing", false, ErrProviderProtocol)
	}
	return identity, nil
}

func transferValidateUploadInfo(
	ctx context.Context,
	provider TransferContractProvider,
	credential Credential,
	identity AccountIdentity,
	report *TransferContractCheckReport,
) error {
	started := time.Now()
	info, err := provider.GetUploadInfo(ctx, credential)
	if err != nil {
		return transferProviderFailure(credential.AccountID+"_upload_info", false, err)
	}
	recordTransferStep(report, "upload_info:"+credential.AccountID, started)
	if info.UserID != identity.ProviderUserID {
		return transferFailure(credential.AccountID+"_upload_info", "identity_mismatch", false, ErrCredentialRejected)
	}
	return nil
}

func transferSearchPlayback(
	ctx context.Context,
	provider TransferContractProvider,
	credential Credential,
	query FileQuery,
	report *TransferContractCheckReport,
) (*File, bool, error) {
	started := time.Now()
	files, err := provider.SearchBySHA1(ctx, credential, query)
	if err != nil {
		return nil, false, transferProviderFailure("playback_search", false, err)
	}
	recordTransferStep(report, "search_playback", started)
	if len(files) > 1 {
		return nil, false, transferFailure("playback_search", "response_invalid", false, ErrProviderProtocol)
	}
	if len(files) == 0 {
		return nil, false, nil
	}
	file := files[0]
	return &file, true, nil
}

func validateTransferRangeHash(result FileRangeHash, byteRange ByteRange) (string, error) {
	expectedBytes := byteRange.End - byteRange.Start + 1
	normalized, err := normalizeSHA1(result.SHA1)
	if err != nil || result.BytesRead != expectedBytes {
		return "", ErrProviderProtocol
	}
	return normalized, nil
}

func boundedContractRange(fileSize int64) ByteRange {
	end := contractCheckRangeBytes - 1
	if fileSize <= contractCheckRangeBytes {
		end = fileSize - 1
	}
	return ByteRange{Start: 0, End: end}
}

func validTransferFile(file *File, expectedSize int64, expectedParentID string) bool {
	return file != nil && strings.TrimSpace(file.ID) != "" && !file.IsDirectory && file.Size == expectedSize && strings.TrimSpace(file.PickCode) != "" &&
		(expectedParentID == "" || file.ParentID == expectedParentID)
}

func transferFailure(stage, code string, fileMayExist bool, err error) error {
	return &transferContractCheckError{stage: stage, code: code, fileMayExist: fileMayExist, err: err}
}

func transferProviderFailure(stage string, fileMayExist bool, err error) error {
	return transferFailure(stage, providerContractErrorCode(err), fileMayExist, err)
}

// recordTransferStep appends one fixed operation name and elapsed duration.
func recordTransferStep(report *TransferContractCheckReport, operation string, started time.Time) {
	report.Steps = append(report.Steps, ContractCheckStepReport{
		Operation: operation, DurationMS: time.Since(started).Milliseconds(),
	})
}

var _ TransferContractProvider = (*CookieProvider)(nil)
