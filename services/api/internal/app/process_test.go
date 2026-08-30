package app

import (
	"errors"
	"reflect"
	"testing"
)

func TestRunProcessExecutesAPILifecycleInOrder(t *testing.T) {
	var calls []string
	appendCall := func(name string) func() error {
		return func() error { calls = append(calls, name); return nil }
	}
	err := runProcess(processDependencies{
		initDB:             func() { calls = append(calls, "init_db") },
		closeDB:            appendCall("close_db"),
		migrate:            appendCall("migrate"),
		verifySchema:       appendCall("verify_schema"),
		syncLogLevel:       func() { calls = append(calls, "sync_log_level") },
		bootstrap:          func() { calls = append(calls, "bootstrap") },
		initJWT:            appendCall("init_jwt"),
		initInternalSecret: appendCall("init_internal_secret"),
		start:              appendCall("start"),
	})
	if err != nil {
		t.Fatalf("runProcess() error = %v", err)
	}
	want := []string{"init_db", "migrate", "verify_schema", "sync_log_level", "bootstrap", "init_jwt", "init_internal_secret", "start", "close_db"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestRunProcessStopsAtFirstFailureAndStillClosesDatabase(t *testing.T) {
	fixtureErr := errors.New("fixture process failure")
	tests := []struct {
		name      string
		failStage string
		wantCalls []string
	}{
		{name: "migrate", failStage: "migrate", wantCalls: []string{"init_db", "migrate", "close_db"}},
		{name: "verify schema", failStage: "verify_schema", wantCalls: []string{"init_db", "migrate", "verify_schema", "close_db"}},
		{name: "jwt", failStage: "init_jwt", wantCalls: []string{"init_db", "migrate", "verify_schema", "sync_log_level", "bootstrap", "init_jwt", "close_db"}},
		{name: "internal secret", failStage: "init_internal_secret", wantCalls: []string{"init_db", "migrate", "verify_schema", "sync_log_level", "bootstrap", "init_jwt", "init_internal_secret", "close_db"}},
		{name: "start", failStage: "start", wantCalls: []string{"init_db", "migrate", "verify_schema", "sync_log_level", "bootstrap", "init_jwt", "init_internal_secret", "start", "close_db"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls []string
			stage := func(name string) func() error {
				return func() error {
					calls = append(calls, name)
					if name == test.failStage {
						return fixtureErr
					}
					return nil
				}
			}
			err := runProcess(processDependencies{
				initDB:             func() { calls = append(calls, "init_db") },
				closeDB:            stage("close_db"),
				migrate:            stage("migrate"),
				verifySchema:       stage("verify_schema"),
				syncLogLevel:       func() { calls = append(calls, "sync_log_level") },
				bootstrap:          func() { calls = append(calls, "bootstrap") },
				initJWT:            stage("init_jwt"),
				initInternalSecret: stage("init_internal_secret"),
				start:              stage("start"),
			})
			if !errors.Is(err, fixtureErr) || !reflect.DeepEqual(calls, test.wantCalls) {
				t.Fatalf("runProcess() = error %v calls %v, want %v", err, calls, test.wantCalls)
			}
		})
	}
}

func TestRunProcessRejectsMissingDependenciesBeforeDatabaseInitialization(t *testing.T) {
	initCalls := 0
	err := runProcess(processDependencies{initDB: func() { initCalls++ }})
	if err == nil || initCalls != 0 {
		t.Fatalf("runProcess() = error %v initCalls=%d", err, initCalls)
	}
}
