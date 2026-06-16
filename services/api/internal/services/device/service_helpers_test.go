package device

import (
	"testing"
	"time"
)

func TestNormalizeClientName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "trim lower collapse spaces", in: "  EMBY   Theater  ", want: "emby theater"},
		{name: "strip numeric version suffix", in: "Infuse 7.8.1", want: "infuse"},
		{name: "strip v version suffix", in: "Fileball v1.2", want: "fileball"},
		{name: "convert full width spaces", in: "VidHub\u3000Pro", want: "vidhub pro"},
		{name: "preserve version inside name", in: "Client 2 Beta", want: "client 2 beta"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeClientName(tt.in); got != tt.want {
				t.Fatalf("normalizeClientName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty(" ", "\t", " value ", "fallback"); got != " value " {
		t.Fatalf("expected first non-empty original value, got %q", got)
	}
	if got := firstNonEmpty(" ", "\t"); got != "" {
		t.Fatalf("expected empty result, got %q", got)
	}
}

func TestParseDateTime(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want time.Time
	}{
		{name: "rfc3339 nano", in: "2026-06-16T12:13:14.123456789Z", want: mustParseTime(t, time.RFC3339Nano, "2026-06-16T12:13:14.123456789Z")},
		{name: "rfc3339", in: "2026-06-16T12:13:14Z", want: mustParseTime(t, time.RFC3339, "2026-06-16T12:13:14Z")},
		{name: "emby ticks format", in: "2026-06-16T12:13:14.0000000Z", want: mustParseTime(t, "2006-01-02T15:04:05.0000000Z", "2026-06-16T12:13:14.0000000Z")},
		{name: "database format", in: "2026-06-16 12:13:14", want: mustParseTime(t, "2006-01-02 15:04:05", "2026-06-16 12:13:14")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseDateTime(tt.in); !got.Equal(tt.want) {
				t.Fatalf("parseDateTime(%q) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
	if got := parseDateTime("not a time"); !got.IsZero() {
		t.Fatalf("expected invalid time to parse as zero, got %s", got)
	}
	if got := parseDateTime(" "); !got.IsZero() {
		t.Fatalf("expected blank time to parse as zero, got %s", got)
	}
}

func TestEnsureDeviceEntry(t *testing.T) {
	entries := map[string]*DeviceItem{}

	first := ensureDeviceEntry(entries, "device_1")
	first.ClientName = "Emby"
	second := ensureDeviceEntry(entries, "device_1")

	if first != second {
		t.Fatalf("expected existing entry to be reused")
	}
	if second.ClientName != "Emby" {
		t.Fatalf("expected existing entry fields to be preserved, got %+v", second)
	}
	if entries["device_1"].DeviceID != "device_1" {
		t.Fatalf("expected device ID to be set, got %+v", entries["device_1"])
	}
}

func mustParseTime(t *testing.T, layout, value string) time.Time {
	t.Helper()

	parsed, err := time.Parse(layout, value)
	if err != nil {
		t.Fatalf("parse fixture %q: %v", value, err)
	}
	return parsed
}
