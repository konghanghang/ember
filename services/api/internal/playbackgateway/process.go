package playbackgateway

import (
	"context"
	"errors"
	"log"
	"os"

	configpkg "github.com/konghang/ember/backend/internal/config"
	dbpkg "github.com/konghang/ember/backend/internal/db"
)

// RunProcess prepares the shared database schema and ConfigService-backed Emby
// settings, then runs the already version-verified gateway until ctx is done.
// It intentionally does not initialize API JWT, Bot secrets, cron or seed data.
func RunProcess(ctx context.Context) error {
	if ctx == nil {
		return ErrRuntimeDependency
	}
	dbpkg.InitDB()
	defer func() {
		if err := dbpkg.Close(); err != nil {
			log.Printf("[PlaybackGateway] code=database_close_failed errorType=%T", err)
		}
	}()
	if err := dbpkg.Migrate(); err != nil {
		return err
	}
	if err := dbpkg.VerifySchema(); err != nil {
		return err
	}
	runtime, err := NewProductionRuntime(ctx, os.Getenv, ProductionDependencies{
		Database: dbpkg.DB,
		Settings: configpkg.NewConfigService(),
		Logger:   log.Default(),
	})
	if err != nil {
		logRuntimeProcessFailure(log.Default(), "runtime_init", err)
		return err
	}
	if err := runtime.Run(ctx); err != nil {
		logRuntimeProcessFailure(log.Default(), "runtime_run", err)
		return err
	}
	return nil
}

// runtimeFailureReasonCode maps every public runtime sentinel to a stable,
// credential-free code suitable for production logs.
func runtimeFailureReasonCode(err error) string {
	switch {
	case errors.Is(err, ErrRuntimeDatabaseURLInvalid):
		return "database_url_invalid"
	case errors.Is(err, ErrRuntimeEncryptionKeyInvalid):
		return "encryption_key_invalid"
	case errors.Is(err, ErrRuntimeEmbyURLUnavailable):
		return "emby_url_unavailable"
	case errors.Is(err, ErrRuntimeEmbyAPIKeyUnavailable):
		return "emby_api_key_unavailable"
	case errors.Is(err, ErrRuntimeConfig):
		return "runtime_config_invalid"
	case errors.Is(err, ErrUpstreamIdentity):
		return "upstream_identity_failed"
	case errors.Is(err, ErrUnsupportedEmbyVersion):
		return "upstream_version_unsupported"
	case errors.Is(err, ErrRuntimeDependency):
		return "runtime_dependency_missing"
	case errors.Is(err, ErrRuntimeListen):
		return "listen_failed"
	case errors.Is(err, ErrRuntimeServe):
		return "serve_failed"
	case errors.Is(err, ErrRuntimeShutdown):
		return "shutdown_failed"
	default:
		return "unknown"
	}
}

// logRuntimeProcessFailure records only fixed stage/reason metadata and the Go
// error type; the original error text may contain URLs or provider details.
func logRuntimeProcessFailure(logger *log.Logger, stage string, err error) {
	if logger == nil {
		logger = log.Default()
	}
	logger.Printf(
		"[PlaybackGateway] code=process_failed stage=%s reasonCode=%s errorType=%T",
		stage,
		runtimeFailureReasonCode(err),
		err,
	)
}
