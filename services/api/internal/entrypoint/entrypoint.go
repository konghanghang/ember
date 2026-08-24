// Package entrypoint dispatches the single ember binary to one process role.
package entrypoint

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	apppkg "github.com/konghang/ember/backend/internal/app"
	envpkg "github.com/konghang/ember/backend/internal/envbootstrap"
	logpkg "github.com/konghang/ember/backend/internal/logging"
	gatewaypkg "github.com/konghang/ember/backend/internal/playbackgateway"
)

const (
	exitSuccess = 0
	exitFailure = 1
	exitUsage   = 2
)

const usage = `Usage: ember [command]

Commands:
  api       Run the Ember API (default)
  gateway   Run the Ember playback gateway
  help      Show this help
`

type dependencies struct {
	loadEnvironment func() envpkg.Result
	initLogging     func(string) error
	logInitialized  func(string)
	runAPI          func() error
	runGateway      func(context.Context) error
}

// Run dispatches one process role and installs a signal context only for the
// Gateway, whose HTTP lifecycle explicitly supports graceful shutdown.
func Run(args []string) int {
	processDependencies := dependencies{
		loadEnvironment: envpkg.Load,
		initLogging:     logpkg.Init,
		logInitialized:  logpkg.LogInitialized,
		runAPI:          apppkg.RunProcess,
		runGateway:      gatewaypkg.RunProcess,
	}
	command, help, err := parseCommand(args)
	if err != nil || help || command != "gateway" {
		// API keeps the operating system's existing signal behavior until its
		// HTTP lifecycle explicitly supports context-driven graceful shutdown.
		return run(context.Background(), args, os.Stdout, os.Stderr, processDependencies)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, args, os.Stdout, os.Stderr, processDependencies)
}

// run is the testable command boundary. Help and usage failures never
// initialize logging, the database, a listener or either process runner.
func run(ctx context.Context, args []string, stdout, stderr io.Writer, dependencies dependencies) int {
	command, help, err := parseCommand(args)
	if help {
		_, _ = io.WriteString(stdout, usage)
		return exitSuccess
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ember: %s\n%s", err.Error(), usage)
		return exitUsage
	}
	if dependencies.initLogging == nil || (command == "api" && dependencies.runAPI == nil) ||
		(command == "gateway" && dependencies.runGateway == nil) {
		_, _ = fmt.Fprintln(stderr, "ember: code=dependency_missing")
		return exitFailure
	}
	environmentResult := envpkg.Result{}
	if dependencies.loadEnvironment != nil {
		environmentResult = dependencies.loadEnvironment()
	}
	if err := dependencies.initLogging(command); err != nil {
		_, _ = fmt.Fprintf(stderr, "ember: command=%s code=logging_init_failed errorType=%T\n", command, err)
		return exitFailure
	}
	logEnvironmentResult(environmentResult)
	if dependencies.logInitialized != nil {
		dependencies.logInitialized(command)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	switch command {
	case "api":
		err = dependencies.runAPI()
	case "gateway":
		err = dependencies.runGateway(ctx)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ember: command=%s code=process_failed errorType=%T\n", command, err)
		return exitFailure
	}
	return exitSuccess
}

// logEnvironmentResult reports only the bounded selected path and error type
// after the configured process writer is ready.
func logEnvironmentResult(result envpkg.Result) {
	if result.Path == "" {
		return
	}
	path := result.Path
	if len(path) > 1024 {
		path = path[:1024]
	}
	if result.Err != nil {
		log.Printf("[Env] code=dotenv_load_failed path=%q errorType=%T", path, result.Err)
		return
	}
	log.Printf("[Env] code=dotenv_loaded path=%q", path)
}

// parseCommand fixes the public CLI contract: no arguments means API, help is
// side-effect free, and every unknown or extra argument fails closed.
func parseCommand(args []string) (command string, help bool, err error) {
	if len(args) == 0 {
		return "api", false, nil
	}
	if len(args) > 1 {
		return "", false, fmt.Errorf("unexpected arguments")
	}
	switch args[0] {
	case "api", "gateway":
		return args[0], false, nil
	case "help", "-h", "--help":
		return "", true, nil
	default:
		return "", false, fmt.Errorf("unknown command %q", args[0])
	}
}
