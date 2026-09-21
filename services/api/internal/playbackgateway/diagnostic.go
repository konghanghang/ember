package playbackgateway

import (
	"context"
	"fmt"
	"hash/maphash"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/konghang/ember/backend/internal/services/embytoken"
)

var diagnosticSeed = maphash.MakeSeed()
var diagnosticSequence atomic.Uint64

type diagnosticRequestKey struct{}

// withDiagnosticRequest assigns a process-scoped ID independent of incoming
// headers, so caller-controlled IDs cannot inject or correlate credentials.
func withDiagnosticRequest(r *http.Request) *http.Request {
	if r == nil {
		return r
	}
	id := fmt.Sprintf("%016x-%x", maphash.String(diagnosticSeed, "request"), diagnosticSequence.Add(1))
	return r.WithContext(context.WithValue(r.Context(), diagnosticRequestKey{}, id))
}

// diagnosticRequestID reads only the internally generated request identifier.
func diagnosticRequestID(ctx context.Context) string {
	id, _ := ctx.Value(diagnosticRequestKey{}).(string)
	return id
}

// diagnosticSessionRef correlates video and event identities within a process;
// neither the original session ID nor the Redis/HMAC key is exposed.
func diagnosticSessionRef(p embytoken.Principal, session string) string {
	if session == "" {
		return "unavailable"
	}
	var h maphash.Hash
	h.SetSeed(diagnosticSeed)
	for _, field := range []string{p.ServerID, p.User.ID, p.MappingID, p.DeviceID, session} {
		if field == "" {
			return "unavailable"
		}
		fmt.Fprintf(&h, "%d:%s", len(field), field)
	}
	return fmt.Sprintf("%016x", h.Sum64())
}

// rangeDiagnostics logs only validated non-negative byte offsets, never raw
// malformed headers. Multi-range and oversized inputs are classified only.
func rangeDiagnostics(header http.Header) string {
	values := header.Values("Range")
	if len(values) == 0 {
		return "rangeKind=none"
	}
	if len(values) != 1 || len(values[0]) > 128 {
		return "rangeKind=invalid"
	}
	value := strings.TrimSpace(values[0])
	if !strings.HasPrefix(value, "bytes=") {
		return "rangeKind=invalid"
	}
	value = strings.TrimPrefix(value, "bytes=")
	if strings.Contains(value, ",") {
		return "rangeKind=multiple"
	}
	parts := strings.Split(value, "-")
	if len(parts) != 2 || (parts[0] == "" && parts[1] == "") {
		return "rangeKind=invalid"
	}
	parse := func(value string) (uint64, error) { return strconv.ParseUint(value, 10, 63) }
	if parts[0] == "" {
		length, err := parse(parts[1])
		if err != nil || length == 0 {
			return "rangeKind=invalid"
		}
		return fmt.Sprintf("rangeKind=suffix rangeLength=%d", length)
	}
	start, err := parse(parts[0])
	if err != nil {
		return "rangeKind=invalid"
	}
	if parts[1] == "" {
		return fmt.Sprintf("rangeKind=open rangeStart=%d", start)
	}
	end, err := parse(parts[1])
	if err != nil || end < start {
		return "rangeKind=invalid"
	}
	return fmt.Sprintf("rangeKind=bounded rangeStart=%d rangeEnd=%d", start, end)
}

// purposeDiagnostic emits a fixed classification, not arbitrary Header text.
func purposeDiagnostic(header http.Header, key string) string {
	values := header.Values(key)
	if len(values) == 0 {
		return "none"
	}
	if len(values) == 1 && strings.EqualFold(strings.TrimSpace(values[0]), "prefetch") {
		return "prefetch"
	}
	return "other"
}

// logVideoRequestStart precedes identity lookup and all upstream work. The
// completed request and final decision share this same diagnostic ID.
func (g *Gateway) logVideoRequestStart(r *http.Request, snapshot requestLogSnapshot) {
	g.debugf("[PlaybackGateway] level=debug code=video_request_started requestId=%s method=%s path=%q %s purpose=%s secPurpose=%s",
		diagnosticRequestID(r.Context()), snapshot.method, snapshot.path, rangeDiagnostics(r.Header),
		purposeDiagnostic(r.Header, "Purpose"), purposeDiagnostic(r.Header, "Sec-Purpose"))
}
