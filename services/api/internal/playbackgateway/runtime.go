package playbackgateway

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	embypkg "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/services/embytoken"
	"gorm.io/gorm"
)

const (
	supportedEmbyVersion       = "4.9.3.0"
	runtimeReadHeaderTimeout   = 5 * time.Second
	runtimeIdleTimeout         = 60 * time.Second
	runtimeShutdownTimeout     = 10 * time.Second
	runtimeMaxHeaderBytes      = 1 << 20
	minimumEncryptionKeyLength = 32
)

var (
	ErrRuntimeConfig          = errors.New("playback gateway runtime configuration invalid")
	ErrRuntimeDependency      = errors.New("playback gateway runtime dependency missing")
	ErrUpstreamIdentity       = errors.New("playback gateway upstream identity failed")
	ErrUnsupportedEmbyVersion = errors.New("playback gateway upstream version unsupported")
	ErrRuntimeListen          = errors.New("playback gateway listen failed")
	ErrRuntimeServe           = errors.New("playback gateway serve failed")
	ErrRuntimeShutdown        = errors.New("playback gateway shutdown failed")
)

// RuntimeSettings resolves the two Emby settings already owned by
// ConfigService without creating a second configuration source.
type RuntimeSettings interface {
	GetString(string) string
}

// ProductionDependencies are process-owned objects that remain injectable for
// fake upstream and handler tests.
type ProductionDependencies struct {
	Database  *gorm.DB
	Settings  RuntimeSettings
	Transport http.RoundTripper
	Logger    *log.Logger
}

type productionConfig struct {
	encryptionKey string
	listenAddress string
	embyURL       string
	embyAPIKey    string
}

type listenFunction func(network, address string) (net.Listener, error)

// Runtime owns the verified HTTP handler and server lifecycle. Construction
// performs no listen operation, so identity/configuration failures occur before
// any client-visible socket exists.
type Runtime struct {
	server          *http.Server
	identity        embypkg.ServerIdentity
	listen          listenFunction
	logger          *log.Logger
	shutdownTimeout time.Duration
}

// NewProductionRuntime loads deployment and ConfigService values, verifies the
// fixed Emby identity, then composes EmbyTokenService and the gateway handler.
func NewProductionRuntime(
	ctx context.Context,
	getenv func(string) string,
	dependencies ProductionDependencies,
) (*Runtime, error) {
	if ctx == nil || dependencies.Database == nil || dependencies.Settings == nil {
		return nil, ErrRuntimeDependency
	}
	config, err := loadProductionConfig(getenv, dependencies.Settings)
	if err != nil {
		return nil, err
	}
	verifier, err := embypkg.NewServerIdentityVerifier(config.embyURL, config.embyAPIKey, dependencies.Transport)
	if err != nil {
		return nil, ErrRuntimeConfig
	}
	identity, err := verifier.Verify(ctx)
	if err != nil {
		return nil, ErrUpstreamIdentity
	}
	if identity.Version != supportedEmbyVersion {
		return nil, ErrUnsupportedEmbyVersion
	}
	tokenService, err := embytoken.NewService(dependencies.Database, config.encryptionKey, identity.ID)
	if err != nil {
		return nil, ErrRuntimeDependency
	}
	upstream, err := url.Parse(config.embyURL)
	if err != nil {
		return nil, ErrRuntimeConfig
	}
	logger := dependencies.Logger
	if logger == nil {
		logger = log.Default()
	}
	gateway, err := New(Config{
		Upstream:     upstream,
		TokenService: tokenService,
		Transport:    dependencies.Transport,
		Logger:       logger,
	})
	if err != nil {
		return nil, ErrRuntimeConfig
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)
	mux.Handle("/", gateway)
	return &Runtime{
		server: &http.Server{
			Addr:              config.listenAddress,
			Handler:           mux,
			ReadHeaderTimeout: runtimeReadHeaderTimeout,
			IdleTimeout:       runtimeIdleTimeout,
			MaxHeaderBytes:    runtimeMaxHeaderBytes,
		},
		identity:        identity,
		listen:          net.Listen,
		logger:          logger,
		shutdownTimeout: runtimeShutdownTimeout,
	}, nil
}

// Handler returns the fully composed mux for in-process contract tests and
// later embedding. It does not start the server.
func (runtime *Runtime) Handler() http.Handler {
	return runtime.server.Handler
}

// ListenAddress returns the validated deployment address without resolving or
// opening it.
func (runtime *Runtime) ListenAddress() string {
	return runtime.server.Addr
}

// ServerIdentity returns the already verified identity used by the Token
// mapping service; calling it never performs a second Emby request.
func (runtime *Runtime) ServerIdentity() embypkg.ServerIdentity {
	return runtime.identity
}

// Run opens the configured listener only after construction succeeded and
// coordinates http.Server shutdown with the supplied process context.
func (runtime *Runtime) Run(ctx context.Context) error {
	if runtime == nil || runtime.server == nil || runtime.listen == nil || runtime.logger == nil || ctx == nil {
		return ErrRuntimeDependency
	}
	if ctx.Err() != nil {
		return nil
	}
	listener, err := runtime.listen("tcp", runtime.server.Addr)
	if err != nil {
		return ErrRuntimeListen
	}
	runtime.logger.Printf("[PlaybackGateway] code=listener_ready version=%s listenAddress=%s serverIdLength=%d",
		runtime.identity.Version, runtime.server.Addr, len(runtime.identity.ID))
	serveDone := make(chan struct{})
	shutdownResult := make(chan error, 1)
	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), runtime.shutdownTimeout)
			defer cancel()
			shutdownResult <- runtime.server.Shutdown(shutdownCtx)
		case <-serveDone:
			shutdownResult <- nil
		}
	}()
	serveErr := runtime.server.Serve(listener)
	close(serveDone)
	shutdownErr := <-shutdownResult
	if shutdownErr != nil {
		return ErrRuntimeShutdown
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) && !errors.Is(serveErr, net.ErrClosed) {
		return ErrRuntimeServe
	}
	return nil
}

// loadProductionConfig validates sensitive deployment values without placing
// any value in returned error text.
func loadProductionConfig(getenv func(string) string, settings RuntimeSettings) (productionConfig, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	databaseURL := getenv("DATABASE_URL")
	config := productionConfig{
		encryptionKey: getenv("CONFIG_ENCRYPTION_KEY"),
		listenAddress: getenv("PLAYBACK_GATEWAY_LISTEN_ADDR"),
		embyURL:       settings.GetString("EMBY_URL"),
		embyAPIKey:    settings.GetString("EMBY_API_KEY"),
	}
	if !validExactNonEmpty(databaseURL) || !validExactNonEmpty(config.encryptionKey) ||
		len(config.encryptionKey) < minimumEncryptionKeyLength || !validListenAddress(config.listenAddress) ||
		!validExactNonEmpty(config.embyURL) || !validExactNonEmpty(config.embyAPIKey) {
		return productionConfig{}, ErrRuntimeConfig
	}
	return config, nil
}

// validListenAddress requires an explicit numeric TCP port and rejects port 0,
// leaving no implicit or randomly selected production listener.
func validListenAddress(address string) bool {
	if !validExactNonEmpty(address) {
		return false
	}
	_, portText, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	port, err := strconv.Atoi(portText)
	return err == nil && port >= 1 && port <= 65535
}

// validExactNonEmpty rejects surrounding whitespace and line injection while
// preserving otherwise opaque configuration values.
func validExactNonEmpty(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\r\n")
}

// healthHandler reports process readiness after construction; it performs no
// database or upstream call and deliberately bypasses Emby Token middleware.
func healthHandler(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(writer, `{"status":"ok"}`)
}
