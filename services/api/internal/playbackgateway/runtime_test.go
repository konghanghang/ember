package playbackgateway

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
)

const (
	fixtureRuntimeDatabaseURL   = "fixture-database-url"
	fixtureRuntimeEncryptionKey = "fixture-runtime-encryption-key-32-bytes"
	fixtureRuntimeAPIKey        = "fixture-runtime-emby-api-key"
	fixtureRuntimeListenAddress = "127.0.0.1:18096"
	fixtureRuntimeEmbyVersion   = "4.9.3.0"
)

func TestNewProductionRuntimeVerifiesIdentityAndBuildsHandlers(t *testing.T) {
	var identityCalls atomic.Int32
	upstream := newRuntimeIdentityServer(t, &identityCalls, fixtureRuntimeEmbyVersion)
	defer upstream.Close()

	var logs bytes.Buffer
	runtime, err := NewProductionRuntime(context.Background(), runtimeEnvironment(nil), ProductionDependencies{
		Database:          &gorm.DB{},
		Settings:          fakeRuntimeSettings{"EMBY_URL": upstream.URL, "EMBY_API_KEY": fixtureRuntimeAPIKey},
		Logger:            log.New(&logs, "", 0),
		DirectPlayService: &fakeDirectPlayService{},
	})
	if err != nil {
		t.Fatalf("NewProductionRuntime() error = %v", err)
	}
	if identityCalls.Load() != 1 {
		t.Fatalf("identity calls = %d, want 1", identityCalls.Load())
	}
	identity := runtime.ServerIdentity()
	if identity.ID != "server-1" || identity.Version != fixtureRuntimeEmbyVersion || identity.ServerName != "Fixture" {
		t.Fatalf("ServerIdentity() = %+v", identity)
	}
	if runtime.ListenAddress() != fixtureRuntimeListenAddress {
		t.Fatalf("ListenAddress() = %q", runtime.ListenAddress())
	}
	if runtime.server.ReadHeaderTimeout != 5*time.Second || runtime.server.IdleTimeout != 60*time.Second || runtime.server.MaxHeaderBytes != 1<<20 {
		t.Fatalf("server limits = readHeader=%s idle=%s maxHeader=%d", runtime.server.ReadHeaderTimeout, runtime.server.IdleTimeout, runtime.server.MaxHeaderBytes)
	}

	health := httptest.NewRecorder()
	runtime.Handler().ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusOK || health.Header().Get("Content-Type") != "application/json" || health.Body.String() != `{"status":"ok"}` {
		t.Fatalf("health = status %d headers=%v body=%q", health.Code, health.Header(), health.Body.String())
	}
	if identityCalls.Load() != 1 {
		t.Fatalf("health triggered identity call: %d", identityCalls.Load())
	}

	protected := httptest.NewRecorder()
	runtime.Handler().ServeHTTP(protected, httptest.NewRequest(http.MethodGet, "/emby/Items/fixture", nil))
	if protected.Code != http.StatusUnauthorized {
		t.Fatalf("protected status = %d, want %d", protected.Code, http.StatusUnauthorized)
	}
	assertRuntimeSecretsAbsent(t, logs.String(), fixtureRuntimeDatabaseURL, fixtureRuntimeEncryptionKey, fixtureRuntimeAPIKey, upstream.URL)
}

func TestNewProductionRuntimeRejectsMissingConfigurationBeforeIdentityRequest(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		settings RuntimeSettings
		database *gorm.DB
		wantErr  error
	}{
		{name: "missing database URL", env: map[string]string{"DATABASE_URL": ""}, settings: validRuntimeSettings(), database: &gorm.DB{}, wantErr: ErrRuntimeConfig},
		{name: "missing encryption key", env: map[string]string{"CONFIG_ENCRYPTION_KEY": ""}, settings: validRuntimeSettings(), database: &gorm.DB{}, wantErr: ErrRuntimeConfig},
		{name: "missing listen address", env: map[string]string{"PLAYBACK_GATEWAY_LISTEN_ADDR": ""}, settings: validRuntimeSettings(), database: &gorm.DB{}, wantErr: ErrRuntimeConfig},
		{name: "invalid listen address", env: map[string]string{"DATABASE_URL": fixtureRuntimeDatabaseURL, "CONFIG_ENCRYPTION_KEY": fixtureRuntimeEncryptionKey, "PLAYBACK_GATEWAY_LISTEN_ADDR": "not-an-address"}, settings: validRuntimeSettings(), database: &gorm.DB{}, wantErr: ErrRuntimeConfig},
		{name: "missing Emby URL", env: runtimeEnvironmentMap(), settings: fakeRuntimeSettings{"EMBY_API_KEY": fixtureRuntimeAPIKey}, database: &gorm.DB{}, wantErr: ErrRuntimeConfig},
		{name: "missing Emby API key", env: runtimeEnvironmentMap(), settings: fakeRuntimeSettings{"EMBY_URL": "http://emby.invalid"}, database: &gorm.DB{}, wantErr: ErrRuntimeConfig},
		{name: "missing database dependency", env: runtimeEnvironmentMap(), settings: validRuntimeSettings(), wantErr: ErrRuntimeDependency},
		{name: "missing settings dependency", env: runtimeEnvironmentMap(), database: &gorm.DB{}, wantErr: ErrRuntimeDependency},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			runtime, err := NewProductionRuntime(context.Background(), runtimeEnvironment(test.env), ProductionDependencies{
				Database: test.database,
				Settings: test.settings,
				Logger:   log.New(&logs, "", 0),
			})
			if runtime != nil || !errors.Is(err, test.wantErr) {
				t.Fatalf("NewProductionRuntime() = (%v, %v), want nil, %v", runtime, err, test.wantErr)
			}
			assertRuntimeSecretsAbsent(t, err.Error(), fixtureRuntimeDatabaseURL, fixtureRuntimeEncryptionKey, fixtureRuntimeAPIKey)
			assertRuntimeSecretsAbsent(t, logs.String(), fixtureRuntimeDatabaseURL, fixtureRuntimeEncryptionKey, fixtureRuntimeAPIKey)
		})
	}
}

func TestSupportedEmbyVersionRange(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{version: "4.9.0.0", want: true},
		{version: "4.9.0.70", want: true},
		{version: "4.9.3.0", want: true},
		{version: "4.9.999.999", want: true},
		{version: "4.8.99.99"},
		{version: "4.10.0.0"},
		{version: "5.0.0.0"},
		{version: "4.9.3"},
		{version: "4.9.3.0-Beta"},
		{version: "04.9.3.0"},
		{version: "4.9.3.0.1"},
		{version: "4.9.-1.0"},
		{version: ""},
	}
	for _, test := range tests {
		t.Run(test.version, func(t *testing.T) {
			if got := isSupportedEmbyVersion(test.version); got != test.want {
				t.Fatalf("isSupportedEmbyVersion(%q) = %t, want %t", test.version, got, test.want)
			}
		})
	}
}

func TestNewProductionRuntimeAppliesEmbyVersionRange(t *testing.T) {
	tests := []struct {
		version string
		wantErr error
	}{
		{version: "4.9.0.0"},
		{version: "4.9.5.0"},
		{version: "4.8.11.0", wantErr: ErrUnsupportedEmbyVersion},
		{version: "4.10.0.0", wantErr: ErrUnsupportedEmbyVersion},
		{version: "4.9.5.0-Beta", wantErr: ErrUnsupportedEmbyVersion},
	}
	for _, test := range tests {
		t.Run(test.version, func(t *testing.T) {
			var identityCalls atomic.Int32
			upstream := newRuntimeIdentityServer(t, &identityCalls, test.version)
			defer upstream.Close()
			runtime, err := NewProductionRuntime(context.Background(), runtimeEnvironment(nil), ProductionDependencies{
				Database:          &gorm.DB{},
				Settings:          fakeRuntimeSettings{"EMBY_URL": upstream.URL, "EMBY_API_KEY": fixtureRuntimeAPIKey},
				Logger:            log.New(io.Discard, "", 0),
				DirectPlayService: &fakeDirectPlayService{},
			})
			if test.wantErr == nil {
				if err != nil || runtime == nil || runtime.ServerIdentity().Version != test.version {
					t.Fatalf("NewProductionRuntime() = (%v, %v), want supported", runtime, err)
				}
			} else if runtime != nil || !errors.Is(err, test.wantErr) {
				t.Fatalf("NewProductionRuntime() = (%v, %v), want nil, %v", runtime, err, test.wantErr)
			}
			if identityCalls.Load() != 1 {
				t.Fatalf("identity calls = %d, want 1", identityCalls.Load())
			}
			if err != nil {
				assertRuntimeSecretsAbsent(t, err.Error(), fixtureRuntimeAPIKey, upstream.URL)
			}
		})
	}
}

func TestRuntimeRunUsesGracefulShutdownWithoutRealListener(t *testing.T) {
	var identityCalls atomic.Int32
	upstream := newRuntimeIdentityServer(t, &identityCalls, fixtureRuntimeEmbyVersion)
	defer upstream.Close()
	runtime, err := NewProductionRuntime(context.Background(), runtimeEnvironment(nil), ProductionDependencies{
		Database:          &gorm.DB{},
		Settings:          fakeRuntimeSettings{"EMBY_URL": upstream.URL, "EMBY_API_KEY": fixtureRuntimeAPIKey},
		Logger:            log.New(io.Discard, "", 0),
		DirectPlayService: &fakeDirectPlayService{},
	})
	if err != nil {
		t.Fatalf("NewProductionRuntime() error = %v", err)
	}

	listener := newBlockingListener()
	listenCalled := make(chan struct{})
	runtime.listen = func(network, address string) (net.Listener, error) {
		if network != "tcp" || address != fixtureRuntimeListenAddress {
			t.Errorf("listen = %s %s", network, address)
		}
		close(listenCalled)
		return listener, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runtime.Run(ctx)
	}()
	<-listenCalled
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not finish after cancellation")
	}
	if !listener.isClosed() {
		t.Fatal("listener was not closed by graceful shutdown")
	}
}

func TestRuntimeRunSanitizesListenFailure(t *testing.T) {
	var identityCalls atomic.Int32
	upstream := newRuntimeIdentityServer(t, &identityCalls, fixtureRuntimeEmbyVersion)
	defer upstream.Close()
	runtime, err := NewProductionRuntime(context.Background(), runtimeEnvironment(nil), ProductionDependencies{
		Database:          &gorm.DB{},
		Settings:          fakeRuntimeSettings{"EMBY_URL": upstream.URL, "EMBY_API_KEY": fixtureRuntimeAPIKey},
		Logger:            log.New(io.Discard, "", 0),
		DirectPlayService: &fakeDirectPlayService{},
	})
	if err != nil {
		t.Fatalf("NewProductionRuntime() error = %v", err)
	}
	runtime.listen = func(string, string) (net.Listener, error) {
		return nil, errors.New("listen failed with " + fixtureRuntimeEncryptionKey)
	}
	if err := runtime.Run(context.Background()); !errors.Is(err, ErrRuntimeListen) {
		t.Fatalf("Run() error = %v, want %v", err, ErrRuntimeListen)
	} else {
		assertRuntimeSecretsAbsent(t, err.Error(), fixtureRuntimeEncryptionKey, fixtureRuntimeListenAddress)
	}
}

func newRuntimeIdentityServer(t *testing.T, calls *atomic.Int32, version string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Header.Get("X-Emby-Token") != fixtureRuntimeAPIKey {
			t.Error("runtime identity API key changed")
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"Id":"server-1","Version":"`+version+`","ServerName":"Fixture"}`)
	}))
}

func runtimeEnvironment(overrides map[string]string) func(string) string {
	values := runtimeEnvironmentMap()
	for key, value := range overrides {
		values[key] = value
	}
	return func(key string) string { return values[key] }
}

func runtimeEnvironmentMap() map[string]string {
	return map[string]string{
		"DATABASE_URL":                 fixtureRuntimeDatabaseURL,
		"CONFIG_ENCRYPTION_KEY":        fixtureRuntimeEncryptionKey,
		"PLAYBACK_GATEWAY_LISTEN_ADDR": fixtureRuntimeListenAddress,
	}
}

func validRuntimeSettings() fakeRuntimeSettings {
	return fakeRuntimeSettings{"EMBY_URL": "http://emby.invalid", "EMBY_API_KEY": fixtureRuntimeAPIKey}
}

type fakeRuntimeSettings map[string]string

func (settings fakeRuntimeSettings) GetString(key string) string {
	return settings[key]
}

type blockingListener struct {
	closed chan struct{}
	once   sync.Once
}

func newBlockingListener() *blockingListener {
	return &blockingListener{closed: make(chan struct{})}
}

func (listener *blockingListener) Accept() (net.Conn, error) {
	<-listener.closed
	return nil, net.ErrClosed
}

func (listener *blockingListener) Close() error {
	listener.once.Do(func() { close(listener.closed) })
	return nil
}

func (listener *blockingListener) Addr() net.Addr {
	return fakeListenerAddress("playback-gateway-test")
}

func (listener *blockingListener) isClosed() bool {
	select {
	case <-listener.closed:
		return true
	default:
		return false
	}
}

type fakeListenerAddress string

func (address fakeListenerAddress) Network() string { return "test" }
func (address fakeListenerAddress) String() string  { return string(address) }

func assertRuntimeSecretsAbsent(t *testing.T, value string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if secret != "" && strings.Contains(value, secret) {
			t.Fatalf("secret %q leaked in %q", secret, value)
		}
	}
}
