package playback

import (
	"context"
	"strings"
	"testing"
)

func TestCleanupOldRankingsRejectsNonPositiveRetainDays(t *testing.T) {
	tests := []struct {
		name       string
		retainDays int
	}{
		{name: "zero", retainDays: 0},
		{name: "negative", retainDays: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			affected, err := CleanupOldRankings(context.Background(), tt.retainDays)
			if err == nil {
				t.Fatalf("expected retainDays validation error")
			}
			if !strings.Contains(err.Error(), "retainDays") {
				t.Fatalf("expected error to mention retainDays, got %v", err)
			}
			if affected != 0 {
				t.Fatalf("expected no rows affected, got %d", affected)
			}
		})
	}
}
