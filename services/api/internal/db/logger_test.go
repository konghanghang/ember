package db

import (
	"bytes"
	"context"
	"reflect"
	"testing"
	"time"

	logpkg "github.com/konghang/ember/backend/internal/logging"
	"gorm.io/gorm"
)

func TestNewGORMLoggerSuppressesBoundValues(t *testing.T) {
	logger := newGORMLogger(&bytes.Buffer{})
	filter, ok := logger.(gorm.ParamsFilter)
	if !ok {
		t.Fatal("database logger must implement gorm.ParamsFilter")
	}

	sql, params := filter.ParamsFilter(
		context.Background(),
		"INSERT INTO p115_accounts (cookie_ciphertext) VALUES (?)",
		"sensitive-ciphertext",
	)
	if sql != "INSERT INTO p115_accounts (cookie_ciphertext) VALUES (?)" {
		t.Fatalf("ParamsFilter() SQL = %q", sql)
	}
	if !reflect.DeepEqual(params, []interface{}(nil)) {
		t.Fatalf("ParamsFilter() exposed bound values: %#v", params)
	}
}

func TestNewGORMLoggerMapsProjectLevel(t *testing.T) {
	ctx := context.Background()
	t.Cleanup(func() { _ = logpkg.ApplyLevel("info") })
	if err := logpkg.ApplyLevel("info"); err != nil {
		t.Fatalf("ApplyLevel(info) error=%v", err)
	}
	var output bytes.Buffer
	logger := newGORMLogger(&output)
	logger.Info(ctx, "normal SQL must stay debug")
	logger.Trace(ctx, time.Now(), func() (string, int64) { return "SELECT info_hidden", 1 }, nil)
	if output.Len() != 0 {
		t.Fatalf("info level emitted GORM info: %q", output.String())
	}

	if err := logpkg.ApplyLevel("debug"); err != nil {
		t.Fatalf("ApplyLevel(debug) error=%v", err)
	}
	logger.Info(ctx, "debug SQL visible")
	logger.Trace(ctx, time.Now(), func() (string, int64) { return "SELECT debug_visible", 1 }, nil)
	if !bytes.Contains(output.Bytes(), []byte("debug SQL visible")) ||
		!bytes.Contains(output.Bytes(), []byte("SELECT debug_visible")) {
		t.Fatalf("runtime debug level did not emit GORM info: %q", output.String())
	}
}
