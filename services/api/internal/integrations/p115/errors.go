package p115

import "errors"

var (
	ErrCredentialRejected      = errors.New("p115 credential rejected")
	ErrDownloadURLExpired      = errors.New("p115 download URL expired")
	ErrDownloadURLIncompatible = errors.New("p115 download URL header mode incompatible")
	ErrDownloadURLNotAllowed   = errors.New("p115 download URL host not allowed")
	ErrInvalidRequest          = errors.New("p115 provider request invalid")
	ErrProviderRejected        = errors.New("p115 provider rejected request")
	ErrProviderUnavailable     = errors.New("p115 provider unavailable")
	ErrProviderProtocol        = errors.New("p115 provider protocol error")
	ErrTargetFileAmbiguous     = errors.New("p115 target file match is ambiguous")
	ErrTargetFileNotVisible    = errors.New("p115 target file not visible before timeout")
)
