package db

import (
	"bytes"
	"context"
	"reflect"
	"testing"

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
