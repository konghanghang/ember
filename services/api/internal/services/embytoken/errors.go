package embytoken

import "errors"

var (
	ErrInvalidInput          = errors.New("emby token input invalid")
	ErrServerMismatch        = errors.New("emby token server mismatch")
	ErrUserNotFound          = errors.New("emby token user not found")
	ErrUserUnavailable       = errors.New("emby token user unavailable")
	ErrUserExpired           = errors.New("emby token user expired")
	ErrIdentityMismatch      = errors.New("emby token identity mismatch")
	ErrTokenNotFound         = errors.New("emby token mapping not found")
	ErrTokenRevoked          = errors.New("emby token mapping revoked")
	ErrTokenIdentityConflict = errors.New("emby token identity conflict")
	ErrStoreUnavailable      = errors.New("emby token store unavailable")
	ErrRevokeReasonInvalid   = errors.New("emby token revoke reason invalid")
)
