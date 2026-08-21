package p115

import "errors"

var (
	ErrCredentialRejected   = errors.New("p115 credential rejected")
	ErrInvalidRequest       = errors.New("p115 provider request invalid")
	ErrProviderRejected     = errors.New("p115 provider rejected request")
	ErrProviderUnavailable  = errors.New("p115 provider unavailable")
	ErrProviderProtocol     = errors.New("p115 provider protocol error")
	ErrTargetFileAmbiguous  = errors.New("p115 target file match is ambiguous")
	ErrTargetFileNotVisible = errors.New("p115 target file not visible before timeout")
)
