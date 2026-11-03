package playbackgateway

import (
	"net"
	"net/http"
	"time"
)

const (
	embyTransportMaxIdleConns        = 128
	embyTransportMaxIdleConnsPerHost = 32
	embyTransportIdleConnTimeout     = 90 * time.Second
	embyTransportDialTimeout         = 5 * time.Second
	embyTransportKeepAlive           = 30 * time.Second
	embyTransportTLSHandshakeTimeout = 5 * time.Second
)

// newEmbyTransport creates an independent Emby connection pool while retaining
// the standard proxy and protocol behavior. It limits connection setup, never
// the lifetime of a video response or WebSocket, and does not mutate globals.
func newEmbyTransport() *http.Transport {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		// An application replacing the global RoundTripper can still inject it
		// explicitly; an owned pool must remain a standard, verifiable transport.
		base = &http.Transport{Proxy: http.ProxyFromEnvironment}
	}
	transport := base.Clone()
	transport.MaxIdleConns = embyTransportMaxIdleConns
	transport.MaxIdleConnsPerHost = embyTransportMaxIdleConnsPerHost
	transport.IdleConnTimeout = embyTransportIdleConnTimeout
	transport.DialContext = (&net.Dialer{
		Timeout: embyTransportDialTimeout, KeepAlive: embyTransportKeepAlive,
	}).DialContext
	transport.TLSHandshakeTimeout = embyTransportTLSHandshakeTimeout
	transport.ForceAttemptHTTP2 = true
	transport.DisableKeepAlives = false
	transport.MaxConnsPerHost = 0
	transport.ResponseHeaderTimeout = 0
	if transport.TLSClientConfig != nil {
		transport.TLSClientConfig.InsecureSkipVerify = false
	}
	return transport
}
