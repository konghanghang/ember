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
	realCheckAcknowledgement = "I_UNDERSTAND_READ_ONLY_REAL_115"
	realCheckTimeout         = 2 * time.Minute
)

type commandConfigError struct {
	code string
}

// Error intentionally excludes environment names and values from output.
func (err *commandConfigError) Error() string {
	return "p115 contract check configuration invalid"
}

// main delegates to a testable command runner and returns only fixed error codes.
func main() {
	os.Exit(runCommand(os.Stdout, os.Stderr, os.Getenv))
}

// runCommand validates the explicit real-call gate, executes the narrow
// read-only checker, and emits only its sanitized JSON report.
func runCommand(stdout, stderr io.Writer, getenv func(string) string) int {
	input, err := loadCommandInput(getenv)
	if err != nil {
		code := "invalid_configuration"
		var configErr *commandConfigError
		if errors.As(err, &configErr) {
			code = configErr.code
		}
		_, _ = fmt.Fprintf(stderr, "p115 contract check refused: code=%s\n", code)
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), realCheckTimeout)
	defer cancel()
	report, err := p115integration.RunReadOnlyContractCheck(
		ctx,
		p115integration.NewCookieProvider(),
		input,
	)
	if err != nil {
		writeContractFailure(stderr, err)
		return 1
	}

	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		_, _ = fmt.Fprintln(stderr, "p115 contract check failed: stage=report code=encode_failed")
		return 1
	}
	return 0
}

// writeContractFailure emits fixed stage/code plus optional sanitized download
// policy evidence without printing an error string or signed URL.
func writeContractFailure(stderr io.Writer, err error) {
	stage, code := p115integration.ContractCheckFailure(err)
	_, _ = fmt.Fprintf(stderr, "p115 contract check failed: stage=%s code=%s\n", stage, code)
	var policyErr *p115integration.DownloadURLPolicyError
	if !errors.As(err, &policyErr) {
		return
	}
	reason := safeDownloadPolicyReason(policyErr.Reason)
	scheme := safeDownloadPolicyScheme(policyErr.Scheme)
	host := safeDownloadPolicyHost(policyErr.Host)
	_, _ = fmt.Fprintf(
		stderr,
		"p115 contract check evidence: reason=%s scheme=%s host=%s\n",
		reason,
		scheme,
		host,
	)
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

// loadCommandInput reads secrets only from process environment variables;
// command-line flags and persistent dotenv files are intentionally unsupported.
func loadCommandInput(getenv func(string) string) (p115integration.ReadOnlyContractCheckInput, error) {
	if getenv == nil {
		return p115integration.ReadOnlyContractCheckInput{}, &commandConfigError{code: "environment_unavailable"}
	}
	if strings.TrimSpace(getenv("CI")) != "" {
		return p115integration.ReadOnlyContractCheckInput{}, &commandConfigError{code: "ci_forbidden"}
	}
	if strings.TrimSpace(getenv("P115_CONTRACT_CHECK_ACK")) != realCheckAcknowledgement {
		return p115integration.ReadOnlyContractCheckInput{}, &commandConfigError{code: "ack_required"}
	}

	required := []string{
		"P115_SOURCE_COOKIE",
		"P115_PLAYBACK_COOKIE",
		"P115_SOURCE_USER_AGENT",
		"P115_PLAYBACK_USER_AGENT",
		"P115_SOURCE_ROOT_ID",
		"P115_SOURCE_RELATIVE_PATH",
		"P115_SOURCE_SIZE",
		"P115_TEST_CLIENT_USER_AGENT",
	}
	values := make(map[string]string, len(required))
	for _, name := range required {
		value := strings.TrimSpace(getenv(name))
		if value == "" {
			return p115integration.ReadOnlyContractCheckInput{}, &commandConfigError{code: strings.ToLower(name) + "_missing"}
		}
		values[name] = value
	}
	if values["P115_SOURCE_COOKIE"] == values["P115_PLAYBACK_COOKIE"] {
		return p115integration.ReadOnlyContractCheckInput{}, &commandConfigError{code: "cookies_same"}
	}
	sourceSize, err := strconv.ParseInt(values["P115_SOURCE_SIZE"], 10, 64)
	if err != nil || sourceSize <= 0 {
		return p115integration.ReadOnlyContractCheckInput{}, &commandConfigError{code: "source_size_invalid"}
	}

	return p115integration.ReadOnlyContractCheckInput{
		SourceCredential: p115integration.Credential{
			AccountID: "source", Cookie: values["P115_SOURCE_COOKIE"], UserAgent: values["P115_SOURCE_USER_AGENT"],
		},
		PlaybackCredential: p115integration.Credential{
			AccountID: "playback", Cookie: values["P115_PLAYBACK_COOKIE"], UserAgent: values["P115_PLAYBACK_USER_AGENT"],
		},
		SourceFile: p115integration.FilePathQuery{
			RootID: values["P115_SOURCE_ROOT_ID"], RelativePath: values["P115_SOURCE_RELATIVE_PATH"], Size: sourceSize,
		},
		TestClientUserAgent: values["P115_TEST_CLIENT_USER_AGENT"],
	}, nil
}
