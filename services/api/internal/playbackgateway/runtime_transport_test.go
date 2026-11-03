package playbackgateway

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
)

// TestRuntimeOwnedTransportSharesIdentityAndProxy checks production assembly
// reuses one owned pool and closes that pool even if serving never starts.
func TestRuntimeOwnedTransportSharesIdentityAndProxy(t *testing.T) {
	var connections atomic.Int32
	closed := make(chan struct{}, 4)
	upstream := newTransportRuntimeServer(t, fixtureRuntimeEmbyVersion, &connections, closed)
	defer upstream.Close()
	runtime, err := NewProductionRuntime(context.Background(), runtimeEnvironment(nil), ProductionDependencies{
		Database: &gorm.DB{}, Settings: fakeRuntimeSettings{"EMBY_URL": upstream.URL, "EMBY_API_KEY": fixtureRuntimeAPIKey},
		Logger: log.New(io.Discard, "", 0), DirectPlayService: &fakeDirectPlayService{},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	if runtime.ownedTransport == nil || runtime.ownedTransport == http.DefaultTransport {
		t.Fatal("runtime did not acquire its own Emby pool")
	}
	response := httptest.NewRecorder()
	runtime.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK || connections.Load() != 1 {
		t.Fatalf("identity/proxy pooling: status=%d connections=%d", response.Code, connections.Load())
	}
	closer := &countingRuntimeCloser{}
	runtime.dependencyCloser = closer
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil || closer.calls.Load() != 1 {
		t.Fatalf("repeated Close returned err=%v dependency calls=%d", err, closer.calls.Load())
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("runtime Close did not release its idle Emby connection")
	}
}

// TestRuntimeInjectedTransportKeepsCallerOwnership preserves fake/custom
// transport identity and never closes resources owned by the embedding caller.
func TestRuntimeInjectedTransportKeepsCallerOwnership(t *testing.T) {
	transport := &ownedTransportFixture{}
	runtime, err := NewProductionRuntime(context.Background(), runtimeEnvironment(nil), ProductionDependencies{
		Database: &gorm.DB{}, Settings: validRuntimeSettings(), Transport: transport,
		Logger: log.New(io.Discard, "", 0), DirectPlayService: &fakeDirectPlayService{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.ownedTransport != nil {
		t.Fatal("runtime adopted an injected transport")
	}
	response := httptest.NewRecorder()
	runtime.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK || transport.calls.Load() != 2 {
		t.Fatalf("injected transport status=%d calls=%d", response.Code, transport.calls.Load())
	}
	if err := runtime.Close(); err != nil || transport.closeCalls.Load() != 0 {
		t.Fatalf("runtime closed caller transport: err=%v closeCalls=%d", err, transport.closeCalls.Load())
	}
	transport.version = "4.10.0.0"
	if _, err := NewProductionRuntime(context.Background(), runtimeEnvironment(nil), ProductionDependencies{
		Database: &gorm.DB{}, Settings: validRuntimeSettings(), Transport: transport,
		Logger: log.New(io.Discard, "", 0), DirectPlayService: &fakeDirectPlayService{},
	}); !errors.Is(err, ErrUnsupportedEmbyVersion) || transport.closeCalls.Load() != 0 {
		t.Fatalf("failed construction changed caller ownership: err=%v closeCalls=%d", err, transport.closeCalls.Load())
	}
}

// TestRuntimeConstructionFailureClosesOwnedTransport protects startup failures
// after the identity request has already returned an idle pooled connection.
func TestRuntimeConstructionFailureClosesOwnedTransport(t *testing.T) {
	var connections atomic.Int32
	closed := make(chan struct{}, 4)
	upstream := newTransportRuntimeServer(t, "4.10.0.0", &connections, closed)
	defer upstream.Close()
	runtime, err := NewProductionRuntime(context.Background(), runtimeEnvironment(nil), ProductionDependencies{
		Database: &gorm.DB{}, Settings: fakeRuntimeSettings{"EMBY_URL": upstream.URL, "EMBY_API_KEY": fixtureRuntimeAPIKey},
		Logger: log.New(io.Discard, "", 0), DirectPlayService: &fakeDirectPlayService{},
	})
	if runtime != nil || !errors.Is(err, ErrUnsupportedEmbyVersion) {
		t.Fatalf("unsupported runtime=%t err=%v", runtime != nil, err)
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("failed construction leaked the owned idle connection")
	}
}

// TestRuntimeListenFailureClosesOwnedTransport keeps Run's early-exit cleanup
// equivalent to normal graceful shutdown without opening a Gateway listener.
func TestRuntimeListenFailureClosesOwnedTransport(t *testing.T) {
	var connections atomic.Int32
	closed := make(chan struct{}, 4)
	upstream := newTransportRuntimeServer(t, fixtureRuntimeEmbyVersion, &connections, closed)
	defer upstream.Close()
	runtime, err := NewProductionRuntime(context.Background(), runtimeEnvironment(nil), ProductionDependencies{
		Database: &gorm.DB{}, Settings: fakeRuntimeSettings{"EMBY_URL": upstream.URL, "EMBY_API_KEY": fixtureRuntimeAPIKey},
		Logger: log.New(io.Discard, "", 0), DirectPlayService: &fakeDirectPlayService{},
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime.listen = func(string, string) (net.Listener, error) { return nil, errors.New("fixture listen failure") }
	if err := runtime.Run(context.Background()); !errors.Is(err, ErrRuntimeListen) {
		t.Fatalf("Run listen failure=%v", err)
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("listen failure leaked the owned idle connection")
	}
}

type ownedTransportFixture struct {
	calls      atomic.Int32
	closeCalls atomic.Int32
	version    string
}

// RoundTrip acts as an in-process Emby identity/proxy fixture without DNS or HTTP.
func (transport *ownedTransportFixture) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.calls.Add(1)
	version := transport.version
	if version == "" {
		version = fixtureRuntimeEmbyVersion
	}
	body := "proxy"
	if request.URL.Path == "/emby/System/Info" {
		body = `{"Id":"server-1","Version":"` + version + `","ServerName":"Fixture"}`
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

// CloseIdleConnections records accidental ownership transfer to the runtime.
func (transport *ownedTransportFixture) CloseIdleConnections() { transport.closeCalls.Add(1) }

// newTransportRuntimeServer exposes only localhost fake identity/proxy responses
// and reports connection lifecycle events for pool sharing and cleanup checks.
func newTransportRuntimeServer(t *testing.T, version string, connections *atomic.Int32, closed chan<- struct{}) *httptest.Server {
	t.Helper()
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/emby/System/Info" {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"Id":"server-1","Version":"`+version+`","ServerName":"Fixture"}`)
			return
		}
		_, _ = io.WriteString(writer, "proxy")
	}))
	upstream.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
		if state == http.StateClosed {
			closed <- struct{}{}
		}
	}
	upstream.Start()
	return upstream
}
