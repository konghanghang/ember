package p115account

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestMapP115AccountConstraintError(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		code       string
		want       error
	}{
		{name: "enabled role", constraint: enabledRoleConstraint, code: "23505", want: ErrRoleAlreadyEnabled},
		{name: "enabled provider user", constraint: enabledProviderUserConstraint, code: "23505", want: ErrProviderUserAlreadyEnabled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &pgconn.PgError{Code: tt.code, ConstraintName: tt.constraint}
			if got := mapP115AccountConstraintError(err); !errors.Is(got, tt.want) {
				t.Fatalf("mapP115AccountConstraintError() = %v, want %v", got, tt.want)
			}
		})
	}

	original := &pgconn.PgError{Code: "23503", ConstraintName: enabledRoleConstraint}
	if got := mapP115AccountConstraintError(original); got != original {
		t.Fatalf("non-unique error changed: %v", got)
	}
	unknown := &pgconn.PgError{Code: "23505", ConstraintName: "unknown_constraint"}
	if got := mapP115AccountConstraintError(unknown); got != unknown {
		t.Fatalf("unknown unique error changed: %v", got)
	}
}

func TestSafeP115AccountStoreErrorDoesNotLogDatabaseDetail(t *testing.T) {
	var logs bytes.Buffer
	previousOutput := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
	})

	err := &pgconn.PgError{
		Code:           "23514",
		ConstraintName: "ck_p115_accounts_required_values",
		Detail:         "failing row contains encrypted-cookie-secret",
	}
	got := safeP115AccountStoreError("create", err)
	if !errors.Is(got, errStoreOperation) {
		t.Fatalf("safeP115AccountStoreError() = %v, want errStoreOperation", got)
	}
	if strings.Contains(logs.String(), "encrypted-cookie-secret") || strings.Contains(got.Error(), "encrypted-cookie-secret") {
		t.Fatalf("database detail leaked: err=%v logs=%q", got, logs.String())
	}
	if !strings.Contains(logs.String(), "code=23514") || !strings.Contains(logs.String(), "constraint=ck_p115_accounts_required_values") {
		t.Fatalf("safe database diagnostics missing: %q", logs.String())
	}
}
