package db

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"gorm.io/gorm"
)

func TestNewGORMLoggerSuppressesBoundValues(t *testing.T) {
	logger := newGORMLogger(&bytes.Buffer{}, false)
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
	var infoOutput bytes.Buffer
	newGORMLogger(&infoOutput, false).Info(ctx, "normal SQL must stay debug")
	if infoOutput.Len() != 0 {
		t.Fatalf("info level emitted GORM info: %q", infoOutput.String())
	}

	var debugOutput bytes.Buffer
	newGORMLogger(&debugOutput, true).Info(ctx, "debug SQL visible")
	if !bytes.Contains(debugOutput.Bytes(), []byte("debug SQL visible")) {
		t.Fatalf("debug level did not emit GORM info: %q", debugOutput.String())
	}
}
