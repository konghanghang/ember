package playbackgateway

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"
)

func TestRuntimeFailureReasonCodeIsStable(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{err: ErrRuntimeDatabaseURLInvalid, want: "database_url_invalid"},
		{err: ErrRuntimeEncryptionKeyInvalid, want: "encryption_key_invalid"},
		{err: ErrRuntimeEmbyURLUnavailable, want: "emby_url_unavailable"},
		{err: ErrRuntimeEmbyAPIKeyUnavailable, want: "emby_api_key_unavailable"},
		{err: ErrRuntimeConfig, want: "runtime_config_invalid"},
		{err: ErrUpstreamIdentity, want: "upstream_identity_failed"},
		{err: ErrUnsupportedEmbyVersion, want: "upstream_version_unsupported"},
		{err: ErrRuntimeDependency, want: "runtime_dependency_missing"},
		{err: ErrRuntimeListen, want: "listen_failed"},
		{err: ErrRuntimeServe, want: "serve_failed"},
		{err: ErrRuntimeShutdown, want: "shutdown_failed"},
		{err: errors.New("unknown"), want: "unknown"},
	}
	for _, test := range tests {
		if got := runtimeFailureReasonCode(test.err); got != test.want {
			t.Fatalf("runtimeFailureReasonCode(%v) = %q, want %q", test.err, got, test.want)
		}
	}
}

func TestLogRuntimeProcessFailureDoesNotExposeErrorText(t *testing.T) {
	const secret = "secret-upstream-url-and-token"
	var logs bytes.Buffer
	logRuntimeProcessFailure(log.New(&logs, "", 0), "runtime_init", errors.New(secret))

	for _, expected := range []string{
		"code=process_failed",
		"stage=runtime_init",
		"reasonCode=unknown",
		"errorType=*errors.errorString",
	} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("logs = %q, want %s", logs.String(), expected)
		}
	}
	if strings.Contains(logs.String(), secret) {
		t.Fatalf("logs leaked error text: %q", logs.String())
	}
}
