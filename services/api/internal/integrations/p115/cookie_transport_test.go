package p115

import (
	"net/http"
	"testing"
)

// TestCookieHTTPClientsShareBoundedTransport keeps every Cookie client on one
// owned pool with more than the default two idle connections per 115 host,
// without changing timeout or the fail-closed redirect policy.
func TestCookieHTTPClientsShareBoundedTransport(t *testing.T) {
	first, second := newCookieHTTPClient(), newCookieHTTPClient()
	transport, ok := first.Transport.(*http.Transport)
	if !ok || transport == nil || second.Transport != first.Transport {
		t.Fatalf("cookie clients transport = %T / shared=%t", first.Transport, second.Transport == first.Transport)
	}
	if transport == http.DefaultTransport {
		t.Fatal("cookie clients must not mutate or reuse the global transport")
	}
	if transport.MaxIdleConnsPerHost != cookieTransportMaxIdleConnsPerHost || transport.DisableKeepAlives || transport.Proxy == nil {
		t.Fatalf("transport idlePerHost=%d keepAlivesDisabled=%t proxy=%t",
			transport.MaxIdleConnsPerHost, transport.DisableKeepAlives, transport.Proxy != nil)
	}
	if transport.TLSClientConfig != nil && transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("cookie transport must verify TLS")
	}
	if first.Timeout != cookieHTTPClientTimeout || first.CheckRedirect == nil {
		t.Fatalf("client timeout=%s redirectPolicy=%t", first.Timeout, first.CheckRedirect != nil)
	}
}
