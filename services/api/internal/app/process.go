package app

import (
	"fmt"
	"log"

	"github.com/konghang/ember/backend/internal/common"
	dbpkg "github.com/konghang/ember/backend/internal/db"
)

type processDependencies struct {
	initDB             func()
	closeDB            func() error
	migrate            func() error
	verifySchema       func() error
	bootstrap          func()
	initJWT            func() error
	initInternalSecret func() error
	start              func() error
}

// RunProcess owns the API process lifecycle shared by the unified ember
// command and the temporary cmd/server compatibility entrypoint.
func RunProcess() error {
	return runProcess(processDependencies{
		initDB:             dbpkg.InitDB,
		closeDB:            dbpkg.Close,
		migrate:            dbpkg.Migrate,
		verifySchema:       dbpkg.VerifySchema,
		bootstrap:          dbpkg.Bootstrap,
		initJWT:            common.InitJWT,
		initInternalSecret: common.InitInternalAPISecret,
		start:              Start,
	})
}

// runProcess keeps the production ordering explicit and injectable for tests;
// every post-connect failure still closes the shared database handle.
func runProcess(dependencies processDependencies) error {
	if dependencies.initDB == nil || dependencies.closeDB == nil || dependencies.migrate == nil ||
		dependencies.verifySchema == nil || dependencies.bootstrap == nil || dependencies.initJWT == nil ||
		dependencies.initInternalSecret == nil || dependencies.start == nil {
		return fmt.Errorf("api process dependency missing")
	}
	dependencies.initDB()
	defer func() {
		if err := dependencies.closeDB(); err != nil {
			log.Printf("[API] code=database_close_failed errorType=%T", err)
		}
	}()
	if err := dependencies.migrate(); err != nil {
		return apiProcessFailure("migration", err)
	}
	if err := dependencies.verifySchema(); err != nil {
		return apiProcessFailure("schema_verification", err)
	}
	dependencies.bootstrap()
	if err := dependencies.initJWT(); err != nil {
		return apiProcessFailure("jwt_initialization", err)
	}
	if err := dependencies.initInternalSecret(); err != nil {
		return apiProcessFailure("internal_secret_initialization", err)
	}
	if err := dependencies.start(); err != nil {
		return apiProcessFailure("http_server", err)
	}
	return nil
}

// apiProcessFailure records only a fixed stage and error type while preserving
// the original error for errors.Is checks and the caller's exit decision.
func apiProcessFailure(stage string, err error) error {
	log.Printf("[API] code=%s_failed errorType=%T", stage, err)
	return fmt.Errorf("api process %s failed: %w", stage, err)
}
