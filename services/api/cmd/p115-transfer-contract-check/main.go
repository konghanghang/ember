package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
)

const (
	transferCheckAcknowledgement = "I_UNDERSTAND_PLAYBACK_FILE_WILL_BE_CREATED_AND_RETAINED"
	transferCheckTimeout         = 3 * time.Minute
)

type commandConfigError struct {
	code string
}

// Error intentionally excludes environment names and values from output.
func (err *commandConfigError) Error() string {
	return "p115 transfer contract check configuration invalid"
}

// main delegates to a testable command runner and never starts a service.
func main() {
	os.Exit(runCommand(os.Stdout, os.Stderr, os.Getenv))
}

// runCommand enforces the retained-write acknowledgement, runs one bounded
// transfer check, and emits only terminal-safe report or failure metadata.
func runCommand(stdout, stderr io.Writer, getenv func(string) string) int {
	input, err := loadCommandInput(getenv)
	if err != nil {
		code := "invalid_configuration"
		var configErr *commandConfigError
		if errors.As(err, &configErr) {
			code = configErr.code
		}
		_, _ = fmt.Fprintf(stderr, "p115 transfer contract check refused: code=%s\n", code)
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), transferCheckTimeout)
	defer cancel()
	report, err := p115integration.RunTransferContractCheck(
		ctx,
		p115integration.NewCookieProvider(),
		input,
	)
	if err != nil {
		writeTransferFailure(stderr, err)
		return 1
	}

	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		_, _ = fmt.Fprintln(stderr, "p115 transfer contract check failed: stage=report code=encode_failed fileMayExist=true cleanupAttempted=false")
		return 1
	}
	return 0
}

// loadCommandInput reads secrets only from current process environment and
// requires a dedicated playback path plus exact retained-write acknowledgement.
func loadCommandInput(getenv func(string) string) (p115integration.TransferContractCheckInput, error) {
	if getenv == nil {
		return p115integration.TransferContractCheckInput{}, &commandConfigError{code: "environment_unavailable"}
	}
	if strings.TrimSpace(getenv("CI")) != "" {
		return p115integration.TransferContractCheckInput{}, &commandConfigError{code: "ci_forbidden"}
	}
	if strings.TrimSpace(getenv("P115_TRANSFER_CONTRACT_CHECK_ACK")) != transferCheckAcknowledgement {
		return p115integration.TransferContractCheckInput{}, &commandConfigError{code: "ack_required"}
	}

	required := []string{
		"P115_SOURCE_COOKIE",
		"P115_PLAYBACK_COOKIE",
		"P115_SOURCE_USER_AGENT",
		"P115_PLAYBACK_USER_AGENT",
		"P115_SOURCE_ROOT_ID",
		"P115_SOURCE_RELATIVE_PATH",
		"P115_SOURCE_SIZE",
		"P115_PLAYBACK_ROOT_ID",
		"P115_PLAYBACK_TARGET_PATH",
		"P115_TEST_CLIENT_USER_AGENT",
	}
	values := make(map[string]string, len(required))
	for _, name := range required {
		value := strings.TrimSpace(getenv(name))
		if value == "" {
			return p115integration.TransferContractCheckInput{}, &commandConfigError{code: strings.ToLower(name) + "_missing"}
		}
		values[name] = value
	}
	if values["P115_SOURCE_COOKIE"] == values["P115_PLAYBACK_COOKIE"] {
		return p115integration.TransferContractCheckInput{}, &commandConfigError{code: "cookies_same"}
	}
	sourceSize, err := strconv.ParseInt(values["P115_SOURCE_SIZE"], 10, 64)
	if err != nil || sourceSize <= 0 {
		return p115integration.TransferContractCheckInput{}, &commandConfigError{code: "source_size_invalid"}
	}

	return p115integration.TransferContractCheckInput{
		SourceCredential: p115integration.Credential{
			AccountID: "source", Cookie: values["P115_SOURCE_COOKIE"], UserAgent: values["P115_SOURCE_USER_AGENT"],
		},
		PlaybackCredential: p115integration.Credential{
			AccountID: "playback", Cookie: values["P115_PLAYBACK_COOKIE"], UserAgent: values["P115_PLAYBACK_USER_AGENT"],
		},
		SourceFile: p115integration.FilePathQuery{
			RootID: values["P115_SOURCE_ROOT_ID"], RelativePath: values["P115_SOURCE_RELATIVE_PATH"],
		},
		ExpectedSourceSize: sourceSize,
		TargetDirectory: p115integration.DirectoryPathQuery{
			RootID: values["P115_PLAYBACK_ROOT_ID"], RelativePath: values["P115_PLAYBACK_TARGET_PATH"],
		},
		TestClientUserAgent: values["P115_TEST_CLIENT_USER_AGENT"],
	}, nil
}

// writeTransferFailure emits fixed failure metadata and optional bounded
// download/upload protocol evidence; it never prints the wrapped error string.
func writeTransferFailure(stderr io.Writer, err error) {
	stage, code, fileMayExist := p115integration.TransferContractCheckFailure(err)
	_, _ = fmt.Fprintf(
		stderr,
		"p115 transfer contract check failed: stage=%s code=%s fileMayExist=%t cleanupAttempted=false\n",
		stage,
		code,
		fileMayExist,
	)
	var policyErr *p115integration.DownloadURLPolicyError
	if errors.As(err, &policyErr) {
		_, _ = fmt.Fprintf(
			stderr,
			"p115 transfer contract check evidence: reason=%s scheme=%s host=%s\n",
			safeDownloadPolicyReason(policyErr.Reason),
			safeDownloadPolicyScheme(policyErr.Scheme),
			safeDownloadPolicyHost(policyErr.Host),
		)
	}
	var protocolErr *p115integration.RapidUploadProtocolError
	if errors.As(err, &protocolErr) {
		_, _ = fmt.Fprintf(
			stderr,
			"p115 transfer contract check evidence: protocolPhase=%s decryptPhase=%s contentType=%s bodyShape=%s bodyBytes=%d\n",
			safeRapidUploadProtocolPhase(protocolErr.Phase),
			safeRapidUploadDecryptPhase(protocolErr.DecryptPhase),
			safeRapidUploadContentType(protocolErr.ContentType),
			safeRapidUploadBodyShape(protocolErr.BodyShape),
			protocolErr.BodyBytes,
		)
	}
}

// safeRapidUploadProtocolPhase limits output to fixed internal upload stages.
func safeRapidUploadProtocolPhase(phase p115integration.RapidUploadProtocolPhase) string {
	switch phase {
	case p115integration.RapidUploadPhasePayloadBuild,
		p115integration.RapidUploadPhaseRequestBuild,
		p115integration.RapidUploadPhaseResponseRead,
		p115integration.RapidUploadPhaseResponseTooLarge,
		p115integration.RapidUploadPhaseResponseDecrypt,
		p115integration.RapidUploadPhaseResponseMap:
		return string(phase)
	default:
		return "unknown"
	}
}

// safeRapidUploadDecryptPhase limits output to fixed response decoder stages.
func safeRapidUploadDecryptPhase(phase p115integration.RapidUploadDecryptPhase) string {
	switch phase {
	case p115integration.RapidUploadDecryptPhaseAES,
		p115integration.RapidUploadDecryptPhaseLZ4:
		return string(phase)
	default:
		return "unknown"
	}
}

// safeRapidUploadContentType limits output to the Adapter's bounded media types.
func safeRapidUploadContentType(contentType string) string {
	switch contentType {
	case "application/json", "application/octet-stream", "text/plain", "other", "unknown", "":
		if contentType == "" {
			return "unknown"
		}
		return contentType
	default:
		return "unknown"
	}
}

// safeRapidUploadBodyShape limits output to fixed structural classifications.
func safeRapidUploadBodyShape(shape string) string {
	switch shape {
	case "json_object", "json_array", "binary", "empty", "":
		if shape == "" {
			return "unknown"
		}
		return shape
	default:
		return "unknown"
	}
}

// safeDownloadPolicyReason limits output to the Adapter's fixed reason enum.
func safeDownloadPolicyReason(reason p115integration.DownloadURLPolicyReason) string {
	switch reason {
	case p115integration.DownloadURLPolicySchemeNotHTTPS,
		p115integration.DownloadURLPolicyUserinfo,
		p115integration.DownloadURLPolicyExplicitPort,
		p115integration.DownloadURLPolicyFragment,
		p115integration.DownloadURLPolicyIPLiteral,
		p115integration.DownloadURLPolicyHostNotAllowed:
		return string(reason)
	default:
		return "unknown"
	}
}

// safeDownloadPolicyScheme prevents control characters or arbitrary schemes.
func safeDownloadPolicyScheme(scheme string) string {
	if scheme == "http" || scheme == "https" || scheme == "other" {
		return scheme
	}
	return "unknown"
}

// safeDownloadPolicyHost accepts only a bounded lowercase ASCII hostname.
func safeDownloadPolicyHost(host string) string {
	if host == "" || len(host) > 253 {
		return "redacted"
	}
	for index := 0; index < len(host); index++ {
		character := host[index]
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') ||
			character == '.' || character == '-' {
			continue
		}
		return "redacted"
	}
	return host
}
