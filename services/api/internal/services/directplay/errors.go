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
)
