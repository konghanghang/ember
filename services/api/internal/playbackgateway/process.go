package playbackgateway

import (
	"context"
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
		return err
	}
	return runtime.Run(ctx)
}
