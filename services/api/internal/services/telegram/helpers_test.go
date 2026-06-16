package telegram

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestNormalizeBotPollingLeaseSeconds(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{name: "below min", in: 1, want: minBotPollingLeaseSec},
		{name: "within range", in: 120, want: 120},
		{name: "above max", in: 999, want: maxBotPollingLeaseSec},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeBotPollingLeaseSeconds(tt.in); got != tt.want {
				t.Fatalf("normalizeBotPollingLeaseSeconds(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestTelegramUniqueViolationDetection(t *testing.T) {
	err := &pgconn.PgError{
		Code:           "23505",
		ConstraintName: "telegram_bind_codes_code_key",
		Detail:         "Key (code)=(123456) already exists.",
	}

	if !isTelegramCodeDuplicateErr(err) {
		t.Fatalf("expected code duplicate error to be detected")
	}
	if !isTelegramUniqueViolation(err, " CODE ") {
		t.Fatalf("expected field matching to trim and ignore case")
	}
	if !isTelegramUniqueViolation(err, "") {
		t.Fatalf("expected empty field to match any unique violation")
	}
	if isTelegramUniqueViolation(err, "telegramid") {
		t.Fatalf("expected different field not to match")
	}
	if isTelegramUniqueViolation(errors.New("boom"), "code") {
		t.Fatalf("expected non-pg error not to match")
	}
	if isTelegramUniqueViolation(&pgconn.PgError{Code: "23503"}, "") {
		t.Fatalf("expected non-unique pg error not to match")
	}
}

func TestGenerateTelegramBindCodeFormat(t *testing.T) {
	for i := 0; i < 20; i++ {
		code := generateTelegramBindCode()
		if len(code) != 6 {
			t.Fatalf("expected six-digit code, got %q", code)
		}
		for _, ch := range code {
			if ch < '0' || ch > '9' {
				t.Fatalf("expected numeric code, got %q", code)
			}
		}
	}
}
