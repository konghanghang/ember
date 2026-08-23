package playbackgateway

import (
	"context"
	"errors"
	"fmt"
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
	p115integration "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/services/directplay"
	"github.com/konghang/ember/backend/internal/services/embytoken"
	"github.com/konghang/ember/backend/internal/services/p115account"
	"gorm.io/gorm"
)

const (
	minimumSupportedEmbyVersion          = "4.9.0.0"
	exclusiveMaximumSupportedEmbyVersion = "4.10.0.0"
	playbackGatewayListenAddress         = ":8081"
	runtimeReadHeaderTimeout             = 5 * time.Second
	runtimeIdleTimeout                   = 60 * time.Second
	runtimeShutdownTimeout               = 10 * time.Second
	runtimeMaxHeaderBytes                = 1 << 20
)

var (
	ErrRuntimeConfig                = errors.New("playback gateway runtime configuration invalid")
	ErrRuntimeDependency            = errors.New("playback gateway runtime dependency missing")
	ErrUpstreamIdentity             = errors.New("playback gateway upstream identity failed")
	ErrUnsupportedEmbyVersion       = errors.New("playback gateway upstream version unsupported")
	ErrRuntimeListen                = errors.New("playback gateway listen failed")
	ErrRuntimeServe                 = errors.New("playback gateway serve failed")
	ErrRuntimeShutdown              = errors.New("playback gateway shutdown failed")
	ErrRuntimeDatabaseURLInvalid    = fmt.Errorf("%w: database URL invalid", ErrRuntimeConfig)
	ErrRuntimeEncryptionKeyInvalid  = fmt.Errorf("%w: encryption key invalid", ErrRuntimeConfig)
	ErrRuntimeEmbyURLUnavailable    = fmt.Errorf("%w: Emby URL unavailable", ErrRuntimeConfig)
	ErrRuntimeEmbyAPIKeyUnavailable = fmt.Errorf("%w: Emby API key unavailable", ErrRuntimeConfig)
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
	// DirectPlayService is injectable for fake-only runtime tests. Production
	// leaves it nil so construction wires the Cookie Provider and account store.
	DirectPlayService DirectPlayService
}

type productionConfig struct {
	encryptionKey string
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
// compatible Emby identity, then composes EmbyTokenService and the gateway handler.
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
	if !isSupportedEmbyVersion(identity.Version) {
		return nil, ErrUnsupportedEmbyVersion
	}
	tokenService, err := embytoken.NewService(dependencies.Database, config.encryptionKey, identity.ID)
	if err != nil {
		return nil, ErrRuntimeDependency
	}
	directPlayService := dependencies.DirectPlayService
	if directPlayService == nil {
		directPlayService, err = newProductionDirectPlayService(dependencies.Database, config.encryptionKey)
		if err != nil {
			return nil, ErrRuntimeDependency
		}
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
		Upstream:          upstream,
		TokenService:      tokenService,
		DirectPlayService: directPlayService,
		Transport:         dependencies.Transport,
		Logger:            logger,
	})
	if err != nil {
		return nil, ErrRuntimeConfig
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)
	mux.Handle("/", gateway)
	return &Runtime{
		server: &http.Server{
			Addr:              playbackGatewayListenAddress,
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

type embyVersion [4]uint32

// isSupportedEmbyVersion accepts canonical four-part numeric versions in the
// configured half-open compatibility interval [4.9.0.0, 4.10.0.0).
func isSupportedEmbyVersion(value string) bool {
	version, ok := parseEmbyVersion(value)
	if !ok {
		return false
	}
	minimum, minimumOK := parseEmbyVersion(minimumSupportedEmbyVersion)
	maximum, maximumOK := parseEmbyVersion(exclusiveMaximumSupportedEmbyVersion)
	return minimumOK && maximumOK && compareEmbyVersion(version, minimum) >= 0 && compareEmbyVersion(version, maximum) < 0
}

// parseEmbyVersion accepts exactly four canonical unsigned decimal components;
// suffixes, whitespace, signs and leading-zero variants are rejected.
func parseEmbyVersion(value string) (embyVersion, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != len(embyVersion{}) {
		return embyVersion{}, false
	}
	var version embyVersion
	for index, part := range parts {
		parsed, err := strconv.ParseUint(part, 10, 32)
		if err != nil || strconv.FormatUint(parsed, 10) != part {
			return embyVersion{}, false
		}
		version[index] = uint32(parsed)
	}
	return version, true
}

// compareEmbyVersion performs lexicographic comparison on four numeric version
// components and returns -1, 0 or 1.
func compareEmbyVersion(left, right embyVersion) int {
	for index := range left {
		switch {
		case left[index] < right[index]:
			return -1
		case left[index] > right[index]:
			return 1
		}
	}
	return 0
}

// newProductionDirectPlayService composes the existing encrypted 115 account
// service, complete Cookie Provider and PostgreSQL-backed transfer service. It
// performs no Provider request and never exposes the encryption key in errors.
func newProductionDirectPlayService(database *gorm.DB, encryptionKey string) (DirectPlayService, error) {
	provider := p115integration.NewCookieProvider()
	accounts, err := p115account.NewService(database, encryptionKey, provider)
	if err != nil {
		return nil, ErrRuntimeDependency
	}
	service, err := directplay.NewService(database, accounts, provider)
	if err != nil {
		return nil, ErrRuntimeDependency
	}
	return service, nil
}

// Handler returns the fully composed mux for in-process contract tests and
// later embedding. It does not start the server.
func (runtime *Runtime) Handler() http.Handler {
	return runtime.server.Handler
}

// ListenAddress returns the fixed Gateway process address without resolving or
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
		embyURL:       settings.GetString("EMBY_URL"),
		embyAPIKey:    settings.GetString("EMBY_API_KEY"),
	}
	if !validExactNonEmpty(databaseURL) {
		return productionConfig{}, ErrRuntimeDatabaseURLInvalid
	}
	if !validExactNonEmpty(config.encryptionKey) {
		return productionConfig{}, ErrRuntimeEncryptionKeyInvalid
	}
	if !validExactNonEmpty(config.embyURL) {
		return productionConfig{}, ErrRuntimeEmbyURLUnavailable
	}
	if !validExactNonEmpty(config.embyAPIKey) {
		return productionConfig{}, ErrRuntimeEmbyAPIKeyUnavailable
	}
	return config, nil
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
