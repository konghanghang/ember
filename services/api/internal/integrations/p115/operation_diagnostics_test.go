package p115

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"testing"
)

// TestRangeDiagnosticIdentifiesValidationFailure preserves exact bounded Range
// validation and reports its failure without exposing signed URLs or bodies.
func TestRangeDiagnosticIdentifiesValidationFailure(t *testing.T) {
	for _, test := range []struct {
		status         int
		header, reason string
	}{
		{200, "bytes 0-9/1024", "range_response_status_invalid"},
		{206, "bytes 1-10/1024", "range_response_content_range_mismatch"},
		{206, "", "range_response_content_range_missing"},
	} {
		adapter := newTestRangeAdapter(t, "1", func(*http.Request) (*http.Response, error) {
			return rangeHTTPResponse(test.status, test.header, []byte("0123456789")), nil
		})
		_, err := adapter.HashFileRange(context.Background(), fixtureCredential(), FileRangeRequest{File: File{ID: "789", PickCode: fixtureDownloadPickCode, Size: 1024}, Range: ByteRange{Start: 0, End: 9}})
		stage, reason, status := FailureDiagnostic(err)
		if !errors.Is(err, ErrProviderProtocol) || stage != "source_range" || reason != test.reason || status != test.status {
			t.Fatalf("stage=%s reason=%s status=%d error=%v", stage, reason, status, err)
		}
	}
}

// TestOperationDiagnosticRedactsNetworkErrors never forwards URL/DNS error text.
func TestOperationDiagnosticRedactsNetworkErrors(t *testing.T) {
	_, o := observeOperation(context.Background(), "source_parent")
	request, _ := http.NewRequest(http.MethodGet, "https://fixture.invalid/?secret=fixture-secret", nil)
	_, err := o.do(diagnosticHTTPFunc(func(*http.Request) (*http.Response, error) {
		return nil, &url.Error{Op: "Get", URL: request.URL.String(), Err: &net.DNSError{Name: "fixture-secret", Err: "fixture-secret"}}
	}), request)
	err = o.finish(err)
	stage, reason, _ := FailureDiagnostic(err)
	if stage != "source_parent" || reason != "dns_error" || strings.Contains(err.Error(), "fixture-secret") {
		t.Fatalf("unsafe diagnostic: %s %s %v", stage, reason, err)
	}
}

type diagnosticHTTPFunc func(*http.Request) (*http.Response, error)

// Do provides an in-memory HTTP boundary; no external request is possible.
func (f diagnosticHTTPFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

// TestNestedOperationFailureKeepsDownloadStage distinguishes decrypt/JSON
// failures from failures in the subsequent source Range request.
func TestNestedOperationFailureKeepsDownloadStage(t *testing.T) {
	_, download := observeOperation(context.Background(), "download_url")
	_, source := observeOperation(context.Background(), "source_range")
	err := source.finish(download.finish(protocolError("download response decrypt failed")))
	stage, reason, _ := FailureDiagnostic(err)
	if stage != "download_url" || reason != "download_response_decrypt_failed" {
		t.Fatalf("lost deepest stage: %s %s", stage, reason)
	}
}

// TestOperationTraceRecordsOnlyTiming exercises transport hooks without a socket.
func TestOperationTraceRecordsOnlyTiming(t *testing.T) {
	ctx, o := observeOperation(context.Background(), "source_parent")
	trace := httptrace.ContextClientTrace(ctx)
	trace.DNSStart(httptrace.DNSStartInfo{Host: "fixture-secret"})
	trace.DNSDone(httptrace.DNSDoneInfo{})
	trace.ConnectStart("tcp", "fixture-secret")
	trace.ConnectDone("tcp", "fixture-secret", nil)
	trace.TLSHandshakeStart()
	trace.TLSHandshakeDone(tls.ConnectionState{}, nil)
	trace.GotConn(httptrace.GotConnInfo{Reused: true})
	trace.WroteRequest(httptrace.WroteRequestInfo{})
	trace.GotFirstResponseByte()
	if o.firstByte <= 0 || o.dns <= 0 || o.connect <= 0 || o.tls <= 0 || !o.reused || o.phase != "response" {
		t.Fatal("missing trace timing")
	}
	if err := o.finish(nil); err != nil {
		t.Fatal(err)
	}
}

// TestDownloadPolicyDiagnosticDoesNotLeakTarget preserves the exact rejection
// category while excluding even sanitized hostname evidence from playback logs.
func TestDownloadPolicyDiagnosticDoesNotLeakTarget(t *testing.T) {
	for _, test := range []struct {
		err  error
		want string
	}{
		{&DownloadURLPolicyError{Reason: DownloadURLPolicyHostNotAllowed, Host: "fixture-secret"}, "download_host_not_allowed"},
		{&DownloadURLPolicyError{Reason: "fixture-secret", Host: "fixture-secret"}, "download_policy_rejected"},
		{ErrDownloadURLExpired, "download_url_expired"},
		{ErrDownloadURLIncompatible, "download_headers_incompatible"},
	} {
		_, o := observeOperation(context.Background(), "download_url")
		err := o.finish(test.err)
		_, reason, _ := FailureDiagnostic(err)
		if reason != test.want || strings.Contains(err.Error(), "fixture-secret") {
			t.Fatalf("unsafe/missing reason: %s %v", reason, err)
		}
	}
}
