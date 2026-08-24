package entrypoint

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/konghang/ember/backend/internal/envbootstrap"
)

func TestRunLoadsEnvironmentBeforeLogging(t *testing.T) {
	events := make([]string, 0, 4)
	exitCode := run(context.Background(), []string{"gateway"}, &bytes.Buffer{}, &bytes.Buffer{}, dependencies{
		loadEnvironment: func() envbootstrap.Result {
			events = append(events, "environment")
			return envbootstrap.Result{Path: ".env"}
		},
		initLogging: func(string) error {
			events = append(events, "logging")
			return nil
		},
		logInitialized: func(string) { events = append(events, "logging_ready") },
		runGateway: func(context.Context) error {
			events = append(events, "gateway")
			return nil
		},
	})
	if exitCode != exitSuccess || !reflect.DeepEqual(events, []string{"environment", "logging", "logging_ready", "gateway"}) {
		t.Fatalf("exit=%d events=%v", exitCode, events)
	}
}

func TestRunDispatchesOnlySelectedProcess(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantAPICall int
		wantGateway int
		wantLogRole string
	}{
		{name: "no arguments default to api", wantAPICall: 1, wantLogRole: "api"},
		{name: "explicit api", args: []string{"api"}, wantAPICall: 1, wantLogRole: "api"},
		{name: "explicit gateway", args: []string{"gateway"}, wantGateway: 1, wantLogRole: "gateway"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			apiCalls := 0
			gatewayCalls := 0
			loggingRoles := make([]string, 0, 1)
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			exitCode := run(context.Background(), test.args, &stdout, &stderr, dependencies{
				initLogging: func(role string) error { loggingRoles = append(loggingRoles, role); return nil },
				runAPI:      func() error { apiCalls++; return nil },
				runGateway:  func(context.Context) error { gatewayCalls++; return nil },
			})
			if exitCode != exitSuccess || len(loggingRoles) != 1 || loggingRoles[0] != test.wantLogRole ||
				apiCalls != test.wantAPICall || gatewayCalls != test.wantGateway {
				t.Fatalf("run(%v) = exit %d logging=%v api=%d gateway=%d", test.args, exitCode, loggingRoles, apiCalls, gatewayCalls)
			}
			if stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("unexpected output: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunHelpAndInvalidArgumentsNeverInitializeProcesses(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantExit   int
		wantOutput string
		stderr     bool
	}{
		{name: "help", args: []string{"help"}, wantExit: exitSuccess, wantOutput: "Usage:"},
		{name: "long help", args: []string{"--help"}, wantExit: exitSuccess, wantOutput: "gateway"},
		{name: "short help", args: []string{"-h"}, wantExit: exitSuccess, wantOutput: "api"},
		{name: "unknown", args: []string{"unknown"}, wantExit: exitUsage, wantOutput: "unknown command", stderr: true},
		{name: "extra arguments", args: []string{"api", "extra"}, wantExit: exitUsage, wantOutput: "unexpected arguments", stderr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			exitCode := run(context.Background(), test.args, &stdout, &stderr, dependencies{
				loadEnvironment: func() envbootstrap.Result { calls++; return envbootstrap.Result{} },
				initLogging:     func(string) error { calls++; return nil },
				runAPI:          func() error { calls++; return nil },
				runGateway:      func(context.Context) error { calls++; return nil },
			})
			if exitCode != test.wantExit || calls != 0 {
				t.Fatalf("run(%v) = exit %d calls=%d", test.args, exitCode, calls)
			}
			output := stdout.String()
			if test.stderr {
				output = stderr.String()
			}
			if !strings.Contains(output, test.wantOutput) {
				t.Fatalf("output = %q, want %q", output, test.wantOutput)
			}
		})
	}
}

func TestRunSanitizesInitializationAndProcessFailures(t *testing.T) {
	secret := "fixture-sensitive-value"
	tests := []struct {
		name            string
		deps            dependencies
		args            []string
		wantRunnerCalls int
	}{
		{
			name: "logging failure",
			deps: dependencies{
				initLogging: func(string) error { return errors.New("logging failed with " + secret) },
				runAPI:      func() error { return nil },
			},
		},
		{
			name: "api failure",
			deps: dependencies{
				initLogging: func(string) error { return nil },
				runAPI:      func() error { return errors.New("api failed with " + secret) },
			},
			wantRunnerCalls: 1,
		},
		{
			name: "gateway failure",
			args: []string{"gateway"},
			deps: dependencies{
				initLogging: func(string) error { return nil },
				runGateway:  func(context.Context) error { return errors.New("gateway failed with " + secret) },
			},
			wantRunnerCalls: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runnerCalls := 0
			dependencies := test.deps
			if dependencies.runAPI != nil {
				runAPI := dependencies.runAPI
				dependencies.runAPI = func() error { runnerCalls++; return runAPI() }
			}
			if dependencies.runGateway != nil {
				runGateway := dependencies.runGateway
				dependencies.runGateway = func(ctx context.Context) error { runnerCalls++; return runGateway(ctx) }
			}
			var stderr bytes.Buffer
			exitCode := run(context.Background(), test.args, &bytes.Buffer{}, &stderr, dependencies)
			if exitCode != exitFailure || runnerCalls != test.wantRunnerCalls || strings.Contains(stderr.String(), secret) ||
				!strings.Contains(stderr.String(), "errorType=") {
				t.Fatalf("run(%v) = exit %d runners=%d stderr=%q", test.args, exitCode, runnerCalls, stderr.String())
			}
		})
	}
}
