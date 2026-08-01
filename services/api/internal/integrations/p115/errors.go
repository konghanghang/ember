package p115

import "errors"

var (
	ErrCredentialRejected  = errors.New("p115 credential rejected")
	ErrProviderUnavailable = errors.New("p115 provider unavailable")
	ErrProviderProtocol    = errors.New("p115 provider protocol error")
)
