package directplay

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// successSQLStub records database round trips without interpreting PostgreSQL;
// real conditional-update semantics are covered by the integration test.
type successSQLStub struct {
	query    string
	args     []driver.NamedValue
	execs    int
	reads    int
	affected int64
	err      error
}

// Connect returns the in-process statement recorder.
func (s *successSQLStub) Connect(context.Context) (driver.Conn, error) { return s, nil }

// Driver avoids global registration of the test SQL driver.
func (s *successSQLStub) Driver() driver.Driver { return s }

// Open implements the fallback driver contract using the same recorder.
func (s *successSQLStub) Open(string) (driver.Conn, error) { return s, nil }

// Close owns no resources outside the enclosing sql.DB.
func (*successSQLStub) Close() error { return nil }

// Prepare rejects unexpected prepared statements in this one-statement path.
func (*successSQLStub) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}

// Begin rejects a surrounding transaction; the UPDATE itself is atomic.
func (*successSQLStub) Begin() (driver.Tx, error) { return nil, errors.New("unexpected transaction") }

// ExecContext records the actual SQL/parameters passed through GORM and sql.DB.
func (s *successSQLStub) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	s.execs++
	s.query = query
	s.args = append([]driver.NamedValue(nil), args...)
	return driver.RowsAffected(s.affected), s.err
}

// QueryContext detects a regression to a separate lookup before the update.
func (s *successSQLStub) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	s.reads++
	return nil, errors.New("unexpected separate read")
}

// TestTouchSucceededUsesOneConditionalStatement fixes the round-trip budget,
// zero-row no-op contract, deterministic task selection and safe error mapping.
func TestTouchSucceededUsesOneConditionalStatement(t *testing.T) {
	for _, name := range []string{"updated", "recent_or_missing", "database_error"} {
		t.Run(name, func(t *testing.T) {
			stub := &successSQLStub{affected: 1}
			if name == "recent_or_missing" {
				stub.affected = 0
			}
			if name == "database_error" {
				stub.err = errors.New("fixture database failure")
			}
			sqlDB := sql.OpenDB(stub)
			t.Cleanup(func() { _ = sqlDB.Close() })
			database, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
			if err != nil {
				t.Fatal(err)
			}
			at := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
			err = (&gormTaskStore{db: database}).TouchSucceeded(context.Background(), "playback", directPlaySourceSHA1, 1024, at)
			if name == "database_error" {
				if err == nil || strings.Contains(err.Error(), "fixture database failure") {
					t.Fatalf("unsafe database error: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if stub.reads != 0 || stub.execs != 1 {
				t.Fatalf("round trips reads=%d writes=%d", stub.reads, stub.execs)
			}
			for _, clause := range []string{"UPDATE playback_transfer_tasks", "ORDER BY created_at DESC, id DESC", "LIMIT 1", "last_accessed_at IS NULL OR last_accessed_at <="} {
				if !strings.Contains(stub.query, clause) {
					t.Fatalf("missing guard %q in %s", clause, stub.query)
				}
			}
			if len(stub.args) != 8 || stub.args[0].Value != at || stub.args[1].Value != at || stub.args[2].Value != "playback" || stub.args[3].Value != directPlaySourceSHA1 || stub.args[4].Value != int64(1024) || stub.args[5].Value != "succeeded" || stub.args[6].Value != "succeeded" || stub.args[7].Value != at.Add(-time.Minute) {
				t.Fatal("wrong account/content/status/time parameter binding")
			}
		})
	}
}
