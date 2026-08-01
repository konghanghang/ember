package p115account

import "errors"

var (
	ErrStoreUnavailable             = errors.New("p115 account store unavailable")
	ErrAccountNotFound              = errors.New("p115 account not found")
	ErrAccountIDRequired            = errors.New("p115 account id is required")
	ErrAccountUnavailable           = errors.New("p115 account is not active")
	ErrInvalidRole                  = errors.New("invalid p115 account role")
	ErrAliasRequired                = errors.New("p115 account alias is required")
	ErrCookieRequired               = errors.New("p115 account cookie is required")
	ErrAppTypeRequired              = errors.New("p115 account app type is required")
	ErrUserAgentRequired            = errors.New("p115 account user agent is required")
	ErrPlaybackTargetParentRequired = errors.New("p115 playback account target parent is required")
	ErrSourceTargetParentUnexpected = errors.New("p115 source account cannot have a target parent")
)
