package p115

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync"
	"time"

	"github.com/konghang/ember/backend/internal/logging"
)

// protocolFailure retains only an adapter-owned fixed reason, never provider text.
type protocolFailure struct{ reason string }

// Error includes only the adapter-owned reason.
func (e *protocolFailure) Error() string { return ErrProviderProtocol.Error() + ": " + e.reason }

// Unwrap preserves the stable protocol sentinel.
func (e *protocolFailure) Unwrap() error { return ErrProviderProtocol }

// operationFailure exposes safe stage diagnostics without retaining raw URLs or errors.
type operationFailure struct {
	cause         error
	stage, reason string
	status        int
}

// Error excludes diagnostic metadata and raw transport details.
func (e *operationFailure) Error() string { return e.cause.Error() }

// Unwrap preserves health and cancellation classification.
func (e *operationFailure) Unwrap() error { return e.cause }

// FailureDetail can only be populated by the adapter's redacted error boundary.
type FailureDetail struct {
	stage, reason string
	status        int
}

// InspectFailureDetail extracts immutable safe metadata for propagation.
func InspectFailureDetail(err error) FailureDetail {
	s, r, h := FailureDiagnostic(err)
	return FailureDetail{s, r, h}
}

// Fields returns fixed stage/reason labels and a numeric HTTP status.
func (d FailureDetail) Fields() (string, string, int) { return d.stage, d.reason, d.status }

// FailureDiagnostic returns adapter-owned metadata for the final playback log.
// Arbitrary external errors are never formatted or inspected as strings.
func FailureDiagnostic(err error) (stage, reason string, status int) {
	var failure *operationFailure
	if errors.As(err, &failure) {
		return failure.stage, failure.reason, failure.status
	}
	return "", "", 0
}

type operationObservation struct {
	mu                                              sync.Mutex
	stage                                           string
	phase                                           string
	started, dnsStarted, connectStarted, tlsStarted time.Time
	dns, connect, tls, firstByte                    time.Duration
	status                                          int
	reused                                          bool
}

// observeOperation instruments one HTTP operation using timing-only callbacks.
// Trace callbacks never retain hosts, addresses, headers, cookies or body data.
func observeOperation(ctx context.Context, stage string) (context.Context, *operationObservation) {
	o := &operationObservation{stage: stage, started: time.Now(), phase: "queued"}
	trace := &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) {
			o.mu.Lock()
			defer o.mu.Unlock()
			o.phase = "dns"
			o.dnsStarted = time.Now()
		},
		DNSDone: func(httptrace.DNSDoneInfo) { o.mu.Lock(); defer o.mu.Unlock(); o.dns += time.Since(o.dnsStarted) },
		ConnectStart: func(string, string) {
			o.mu.Lock()
			defer o.mu.Unlock()
			o.phase = "connect"
			if o.connectStarted.IsZero() {
				o.connectStarted = time.Now()
			}
		},
		ConnectDone: func(string, string, error) {
			o.mu.Lock()
			defer o.mu.Unlock()
			o.connect = time.Since(o.connectStarted)
		},
		TLSHandshakeStart: func() { o.mu.Lock(); defer o.mu.Unlock(); o.phase = "tls"; o.tlsStarted = time.Now() },
		TLSHandshakeDone:  func(tls.ConnectionState, error) { o.mu.Lock(); defer o.mu.Unlock(); o.tls += time.Since(o.tlsStarted) },
		GotConn: func(info httptrace.GotConnInfo) {
			o.mu.Lock()
			defer o.mu.Unlock()
			o.phase = "request"
			o.reused = info.Reused
		},
		WroteRequest:         func(httptrace.WroteRequestInfo) { o.mu.Lock(); defer o.mu.Unlock(); o.phase = "waiting_headers" },
		GotFirstResponseByte: func() { o.mu.Lock(); defer o.mu.Unlock(); o.phase = "response"; o.firstByte = time.Since(o.started) },
	}
	return httptrace.WithClientTrace(ctx, trace), o
}

// do classifies transport errors before redaction erases their concrete types.
func (o *operationObservation) do(client httpDoer, request *http.Request) (*http.Response, error) {
	response, err := client.Do(request)
	if response != nil {
		o.mu.Lock()
		o.status = response.StatusCode
		o.mu.Unlock()
	}
	if err == nil {
		return response, nil
	}
	reason := "transport_error"
	var dns *net.DNSError
	var network net.Error
	switch {
	case request.Context().Err() != nil:
		reason := "request_canceled"
		if errors.Is(request.Context().Err(), context.DeadlineExceeded) {
			reason = "request_deadline"
		}
		return response, &operationFailure{cause: request.Context().Err(), stage: o.stage, reason: reason}
	case errors.As(err, &dns):
		reason = "dns_error"
	case errors.As(err, &network) && network.Timeout():
		reason = "transport_timeout"
	}
	return response, &operationFailure{cause: ErrProviderUnavailable, stage: o.stage, reason: reason}
}

// finish logs only fixed labels and numbers, preserving the deepest failed stage.
func (o *operationObservation) finish(err error) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	reason := "none"
	if err != nil {
		var protocol *protocolFailure
		var policy *DownloadURLPolicyError
		switch {
		case errors.As(err, &policy):
			reason = "download_policy_rejected"
			switch policy.Reason {
			case DownloadURLPolicySchemeNotHTTPS, DownloadURLPolicyUserinfo, DownloadURLPolicyExplicitPort, DownloadURLPolicyFragment, DownloadURLPolicyIPLiteral, DownloadURLPolicyHostNotAllowed:
				reason = "download_" + string(policy.Reason)
			}
		case errors.Is(err, ErrDownloadURLExpired):
			reason = "download_url_expired"
		case errors.Is(err, ErrDownloadURLIncompatible):
			reason = "download_headers_incompatible"
		case errors.As(err, &protocol):
			reason = strings.NewReplacer(" ", "_", "-", "_").Replace(protocol.reason)
		case errors.Is(err, ErrProviderRejected):
			reason = "provider_rejected"
		case errors.Is(err, ErrCredentialRejected):
			reason = "credential_rejected"
		case errors.Is(err, context.DeadlineExceeded):
			reason = "request_deadline"
		case errors.Is(err, context.Canceled):
			reason = "request_canceled"
		case o.status != 0 && (o.status < 200 || o.status >= 300):
			reason = "http_status"
		default:
			reason = "operation_failed"
		}
		if _, existing, _ := FailureDiagnostic(err); existing != "" {
			reason = existing
		} else {
			err = &operationFailure{cause: err, stage: o.stage, reason: reason, status: o.status}
		}
	}
	// Use one completed-operation line rather than per-hook log spam.
	logging.Debugf("[P115] code=provider_operation_completed stage=%s reason=%s networkPhase=%s statusCode=%d dnsMs=%d connectMs=%d tlsMs=%d firstByteMs=%d connectionReused=%t durationMs=%d success=%t",
		o.stage, reason, o.phase, o.status, o.dns.Milliseconds(), o.connect.Milliseconds(), o.tls.Milliseconds(), o.firstByte.Milliseconds(), o.reused, time.Since(o.started).Milliseconds(), err == nil)
	return err
}

// cookieGETStage uses a fixed endpoint allowlist; it never logs a request URL.
func cookieGETStage(path string) string {
	switch path {
	case "/files/get_path_id":
		return "source_parent"
	case "/files":
		return "source_list"
	default:
		return "cookie_get"
	}
}
