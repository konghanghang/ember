package p115

import (
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	cookieTransportMaxIdleConns        = 64
	cookieTransportMaxIdleConnsPerHost = 16
	cookieTransportIdleConnTimeout     = 90 * time.Second
	cookieTransportDialTimeout         = 5 * time.Second
	cookieTransportKeepAlive           = 30 * time.Second
	cookieTransportTLSHandshakeTimeout = 5 * time.Second
)

var (
	cookieTransportOnce sync.Once
	cookieTransport     *http.Transport
)

// sharedCookieTransport returns one process-wide 115 connection pool. The
// Cookie provider, adapter and validator all talk to the same few 115 hosts;
// the default transport keeps only two idle connections per host, so
// concurrent playback starts would otherwise keep re-dialing TLS.
func sharedCookieTransport() *http.Transport {
	cookieTransportOnce.Do(func() {
		cookieTransport = newCookieTransport()
	})
	return cookieTransport
}

// newCookieTransport clones the standard transport to keep proxy and HTTP/2
// behavior, and bounds only connection setup, never a response body read.
func newCookieTransport() *http.Transport {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		base = &http.Transport{Proxy: http.ProxyFromEnvironment}
	}
	transport := base.Clone()
	transport.MaxIdleConns = cookieTransportMaxIdleConns
	transport.MaxIdleConnsPerHost = cookieTransportMaxIdleConnsPerHost
	transport.IdleConnTimeout = cookieTransportIdleConnTimeout
	transport.DialContext = (&net.Dialer{
		Timeout: cookieTransportDialTimeout, KeepAlive: cookieTransportKeepAlive,
	}).DialContext
	transport.TLSHandshakeTimeout = cookieTransportTLSHandshakeTimeout
	transport.ForceAttemptHTTP2 = true
	transport.DisableKeepAlives = false
	if transport.TLSClientConfig != nil {
		transport.TLSClientConfig.InsecureSkipVerify = false
	}
	return transport
}
