package directplay

import "errors"

var (
	ErrInvalidRequest         = errors.New("direct play request invalid")
	ErrAccountUnavailable     = errors.New("direct play 115 account unavailable")
	ErrAccountsSame           = errors.New("direct play source and playback accounts must differ")
	ErrProviderUnavailable    = errors.New("direct play provider unavailable")
	ErrProviderProtocol       = errors.New("direct play provider protocol invalid")
	ErrRapidUploadUnavailable = errors.New("direct play rapid upload unavailable")
	ErrTargetUnavailable      = errors.New("direct play target unavailable")
	ErrDownloadIncompatible   = errors.New("direct play download incompatible")
	ErrStoreUnavailable       = errors.New("direct play transfer store unavailable")
	ErrLockUnavailable        = errors.New("direct play transfer lock unavailable")
	ErrPathNotMapped          = errors.New("direct play media path not mapped")
)

// FailureContext exposes only fixed, non-secret diagnostics needed by the
// Gateway to identify which DirectPlay boundary failed. Provider responses,
// credentials and signed URLs must never be stored in this context.
type FailureContext struct {
	ProviderOperation string
	AccountRole       string
}

const (
	failureOperationResolveSourcePath    = "resolve_source_path"
	failureOperationHashSourcePreID      = "hash_source_preid"
	failureOperationRapidUpload          = "rapid_upload"
	failureOperationHashSourceChallenge  = "hash_source_challenge"
	failureOperationRapidUploadRetry     = "rapid_upload_retry"
	failureOperationVerifyPlaybackTarget = "verify_playback_target"
	failureOperationSearchPlaybackTarget = "search_playback_target"
	failureOperationGetDownloadURL       = "get_download_url"
)

type failureContextCarrier interface {
	DirectPlayFailureContext() FailureContext
}

type failureContextError struct {
	cause   error
	context FailureContext
}

func (failure *failureContextError) Error() string {
	switch {
	case failure.context.ProviderOperation != "":
		return failure.cause.Error() + ": " + failure.context.ProviderOperation
	case failure.context.AccountRole != "":
		return failure.cause.Error() + ": " + failure.context.AccountRole
	default:
		return failure.cause.Error()
	}
}

func (failure *failureContextError) Unwrap() error {
	return failure.cause
}

func (failure *failureContextError) DirectPlayFailureContext() FailureContext {
	return failure.context
}

// InspectFailure returns the safe diagnostic context carried by a DirectPlay
// error. Errors without an annotated boundary return an empty context.
func InspectFailure(err error) FailureContext {
	var carrier failureContextCarrier
	if errors.As(err, &carrier) {
		return safeFailureContext(carrier.DirectPlayFailureContext())
	}
	return FailureContext{}
}

func withFailureContext(err error, context FailureContext) error {
	if err == nil {
		return nil
	}
	context = safeFailureContext(context)
	if context.ProviderOperation == "" && context.AccountRole == "" {
		return err
	}
	return &failureContextError{cause: err, context: context}
}

func safeFailureContext(context FailureContext) FailureContext {
	switch context.ProviderOperation {
	case failureOperationResolveSourcePath,
		failureOperationHashSourcePreID,
		failureOperationRapidUpload,
		failureOperationHashSourceChallenge,
		failureOperationRapidUploadRetry,
		failureOperationVerifyPlaybackTarget,
		failureOperationSearchPlaybackTarget,
		failureOperationGetDownloadURL:
	default:
		context.ProviderOperation = ""
	}
	if context.AccountRole != "source" && context.AccountRole != "playback" {
		context.AccountRole = ""
	}
	return context
}
