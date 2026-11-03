package playbackgateway

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestEmbyTransportIsIndependent preserves process-wide defaults and the
// transport features needed by normal proxying and long-lived responses.
func TestEmbyTransportIsIndependent(t *testing.T) {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t.Fatal("fixture expects the standard default transport")
	}
	beforeIdle, beforeHostIdle, beforeTLS := base.MaxIdleConns, base.MaxIdleConnsPerHost, base.TLSHandshakeTimeout
	first, second := newEmbyTransport(), newEmbyTransport()
	t.Cleanup(first.CloseIdleConnections)
	t.Cleanup(second.CloseIdleConnections)
	if first == second || first == base {
		t.Fatal("Gateway transports share their connection pool")
	}
	if base.MaxIdleConns != beforeIdle || base.MaxIdleConnsPerHost != beforeHostIdle || base.TLSHandshakeTimeout != beforeTLS {
		t.Fatal("Gateway construction changed the process default transport")
	}
	if first.MaxIdleConns != 128 || first.MaxIdleConnsPerHost != 32 || first.IdleConnTimeout != 90*time.Second || first.TLSHandshakeTimeout != 5*time.Second {
		t.Fatal("Gateway idle pool or connection setup budget is missing")
	}
	if first.Proxy == nil || first.DialContext == nil || !first.ForceAttemptHTTP2 || first.DisableKeepAlives || first.MaxConnsPerHost != 0 || first.ResponseHeaderTimeout != 0 {
		t.Fatal("Gateway transport changed streaming, proxy or concurrency defaults")
	}
	if first.TLSClientConfig != nil && first.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("Gateway transport disabled upstream TLS verification")
	}
}

// TestEmbyTransportRetainsBurstConnections measures actual localhost fake
// upstream connections across identical bursts rather than only pool fields.
func TestEmbyTransportRetainsBurstConnections(t *testing.T) {
	const burstSize = 8
	measure := func(t *testing.T, transport *http.Transport) int32 {
		t.Helper()
		t.Cleanup(transport.CloseIdleConnections)
		var connections, phase atomic.Int32
		entered := make(chan int32, burstSize)
		release := []chan struct{}{make(chan struct{}), make(chan struct{})}
		upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			current := phase.Load()
			entered <- current
			select {
			case <-release[current]:
			case <-request.Context().Done():
				return
			}
			_, _ = io.WriteString(writer, "image")
		}))
		upstream.Config.ConnState = func(_ net.Conn, state http.ConnState) {
			if state == http.StateNew {
				connections.Add(1)
			}
		}
		upstream.Start()
		defer upstream.Close()
		client := &http.Client{Transport: transport}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for batch := range 2 {
			phase.Store(int32(batch))
			var wait sync.WaitGroup
			errors := make(chan error, burstSize)
			for range burstSize {
				wait.Add(1)
				go func() {
					defer wait.Done()
					request, _ := http.NewRequestWithContext(ctx, http.MethodGet, upstream.URL, nil)
					response, err := client.Do(request)
					if err != nil {
						errors <- err
						return
					}
					defer response.Body.Close()
					body, err := io.ReadAll(response.Body)
					if err != nil || string(body) != "image" {
						errors <- fmt.Errorf("fake response body=%q err=%v", body, err)
					}
				}()
			}
			for range burstSize {
				select {
				case <-entered:
				case <-ctx.Done():
					close(release[batch])
					wait.Wait()
					t.Fatal("fake upstream did not receive the complete burst")
				}
			}
			close(release[batch])
			wait.Wait()
			close(errors)
			for err := range errors {
				t.Fatal(err)
			}
			if batch == 0 && connections.Load() != burstSize {
				t.Fatalf("initial concurrent burst opened %d connections, want %d", connections.Load(), burstSize)
			}
		}
		return connections.Load() - burstSize
	}
	var baseline, dedicated int32
	t.Run("standard pool", func(t *testing.T) { baseline = measure(t, http.DefaultTransport.(*http.Transport).Clone()) })
	t.Run("dedicated pool", func(t *testing.T) { dedicated = measure(t, newEmbyTransport()) })
	if baseline <= dedicated || dedicated != 0 {
		t.Fatalf("second burst opened baseline=%d dedicated=%d connections, want reduced reconnects and dedicated=0", baseline, dedicated)
	}
	t.Logf("fake upstream: second 8-request burst opened default=%d dedicated=%d new connections", baseline, dedicated)
}

// TestEmbyTransportStreamsAndCancels checks that the first chunk arrives before
// upstream completion and that caller cancellation terminates the same stream.
func TestEmbyTransportStreamsAndCancels(t *testing.T) {
	finished := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer close(finished)
		_, _ = io.WriteString(writer, "first")
		writer.(http.Flusher).Flush()
		<-request.Context().Done()
	}))
	defer upstream.Close()
	transport := newEmbyTransport()
	t.Cleanup(transport.CloseIdleConnections)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, upstream.URL, nil)
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	first := make([]byte, len("first"))
	if _, err := io.ReadFull(response.Body, first); err != nil || string(first) != "first" {
		t.Fatalf("initial streaming chunk=%q err=%v", first, err)
	}
	cancel()
	if _, err := response.Body.Read(make([]byte, 1)); err == nil {
		t.Fatal("canceled stream kept waiting for more bytes")
	}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("caller cancellation did not reach the fake upstream")
	}
}

// TestEmbyTransportPreservesUpgrade verifies the transport keeps the duplex
// response body required by ReverseProxy for an HTTP/1.1 WebSocket upgrade.
func TestEmbyTransportPreservesUpgrade(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, buffered, err := writer.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer connection.Close()
		_, _ = buffered.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
		_ = buffered.Flush()
		message := make([]byte, 4)
		if _, err := io.ReadFull(buffered, message); err != nil {
			t.Error(err)
			return
		}
		_, _ = buffered.WriteString(strings.ToUpper(string(message)))
		_ = buffered.Flush()
	}))
	defer upstream.Close()
	transport := newEmbyTransport()
	t.Cleanup(transport.CloseIdleConnections)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, upstream.URL, nil)
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "websocket")
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	stream, ok := response.Body.(io.ReadWriteCloser)
	if response.StatusCode != http.StatusSwitchingProtocols || !ok {
		t.Fatalf("upgrade returned status=%d duplex=%t", response.StatusCode, ok)
	}
	if _, err := io.WriteString(stream, "ping"); err != nil {
		t.Fatal(err)
	}
	message := make([]byte, 4)
	if _, err := io.ReadFull(stream, message); err != nil || string(message) != "PING" {
		t.Fatalf("upgraded stream read=%q err=%v", message, err)
	}
}
