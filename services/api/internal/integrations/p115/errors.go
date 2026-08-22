package p115

import "errors"

// DownloadURLPolicyReason identifies which non-secret URL boundary rejected a
// Provider download target.
type DownloadURLPolicyReason string

const (
	DownloadURLPolicySchemeNotHTTPS DownloadURLPolicyReason = "scheme_not_https"
	DownloadURLPolicyUserinfo       DownloadURLPolicyReason = "userinfo_present"
	DownloadURLPolicyExplicitPort   DownloadURLPolicyReason = "explicit_port"
	DownloadURLPolicyFragment       DownloadURLPolicyReason = "fragment_present"
	DownloadURLPolicyIPLiteral      DownloadURLPolicyReason = "ip_literal"
	DownloadURLPolicyHostNotAllowed DownloadURLPolicyReason = "host_not_allowed"
)

// DownloadURLPolicyError retains only sanitized scheme/hostname evidence while
// preserving errors.Is(err, ErrDownloadURLNotAllowed).
type DownloadURLPolicyError struct {
	Reason DownloadURLPolicyReason
	Scheme string
	Host   string
}

// RapidUploadProtocolPhase identifies a fixed internal upload stage without
// exposing Provider response content.
type RapidUploadProtocolPhase string

const (
	RapidUploadPhasePayloadBuild     RapidUploadProtocolPhase = "payload_build"
	RapidUploadPhaseRequestBuild     RapidUploadProtocolPhase = "request_build"
	RapidUploadPhaseResponseRead     RapidUploadProtocolPhase = "response_read"
	RapidUploadPhaseResponseTooLarge RapidUploadProtocolPhase = "response_too_large"
	RapidUploadPhaseResponseDecrypt  RapidUploadProtocolPhase = "response_decrypt"
	RapidUploadPhaseResponseMap      RapidUploadProtocolPhase = "response_map"
)

// RapidUploadDecryptPhase identifies a bounded upload response decoder stage.
type RapidUploadDecryptPhase string

const (
	RapidUploadDecryptPhaseAES RapidUploadDecryptPhase = "aes"
	RapidUploadDecryptPhaseLZ4 RapidUploadDecryptPhase = "lz4"
)

// RapidUploadProtocolError carries only bounded response metadata for the
// one-shot checker while preserving ErrProviderProtocol.
type RapidUploadProtocolError struct {
	Phase        RapidUploadProtocolPhase
	DecryptPhase RapidUploadDecryptPhase
	ContentType  string
	BodyShape    string
	BodyBytes    int
}

// Error intentionally omits phase metadata and Provider response content.
func (err *RapidUploadProtocolError) Error() string {
	return ErrProviderProtocol.Error()
}

// Unwrap preserves errors.Is(err, ErrProviderProtocol).
func (err *RapidUploadProtocolError) Unwrap() error {
	return ErrProviderProtocol
}

// Error intentionally omits URL components from ordinary logs and API errors.
func (err *DownloadURLPolicyError) Error() string {
	return ErrDownloadURLNotAllowed.Error()
}

// Unwrap preserves the existing stable sentinel contract.
func (err *DownloadURLPolicyError) Unwrap() error {
	return ErrDownloadURLNotAllowed
}

var (
	ErrCredentialRejected      = errors.New("p115 credential rejected")
	ErrDownloadURLExpired      = errors.New("p115 download URL expired")
	ErrDownloadURLIncompatible = errors.New("p115 download URL header mode incompatible")
	ErrDownloadURLNotAllowed   = errors.New("p115 download URL host not allowed")
	ErrDirectoryAmbiguous      = errors.New("p115 directory path is ambiguous")
	ErrDirectoryNotFound       = errors.New("p115 directory not found")
	ErrInvalidRequest          = errors.New("p115 provider request invalid")
	ErrProviderRejected        = errors.New("p115 provider rejected request")
	ErrProviderUnavailable     = errors.New("p115 provider unavailable")
	ErrProviderProtocol        = errors.New("p115 provider protocol error")
	ErrSourceDirectoryTooLarge = errors.New("p115 source directory exceeds resolve limit")
	ErrSourceFileAmbiguous     = errors.New("p115 source file match is ambiguous")
	ErrSourceFileNotFound      = errors.New("p115 source file not found")
	ErrTargetFileAmbiguous     = errors.New("p115 target file match is ambiguous")
	ErrTargetFileNotVisible    = errors.New("p115 target file not visible before timeout")
)
