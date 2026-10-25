package directplay

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	p115 "github.com/konghang/ember/backend/internal/integrations/p115"
)

// TestGatewayDiagnosticContractTimingProducer exports actual formatter output
// for the separate Python contract runner; ordinary Go tests need no Python.
func TestGatewayDiagnosticContractTimingProducer(t *testing.T) {
	type timingCase struct {
		Name   string   `json:"name"`
		Fields []string `json:"fields"`
	}
	packet := struct {
		Kind  string       `json:"kind"`
		Cases []timingCase `json:"cases"`
	}{Kind: "timing"}
	media, download := newMediaResolutionCache(1), newDownloadURLCache(1)
	cacheTime := time.Unix(1, 0)
	for index, name := range []string{"miss", "hit", "bypass"} {
		ctx, recorder := withTiming(context.Background())
		now := cacheTime
		recorder.started, recorder.now = now, func() time.Time { return now }
		now = now.Add(3 * time.Millisecond)
		finishPreparation(ctx)
		// Traverse the production stage list: adding a Go timing stage must
		// also keep that stage in the Python filter and summary.
		for stageIndex, stage := range timingStages {
			if stage == "prepare" {
				continue
			}
			calls := 1
			if stage == "preIDWait" {
				calls = 2
			}
			for call := 0; call < calls; call++ {
				finish := measureStage(ctx, stage)
				now = now.Add(time.Duration(stageIndex+1) * time.Millisecond)
				finish()
			}
		}
		now = now.Add(7 * time.Millisecond)
		mediaReason, downloadReason := "", ""
		switch name {
		case "miss":
			mediaReason, downloadReason = "dependent_url_expired", "forced_refresh"
		case "bypass":
			mediaReason, downloadReason = "key_unavailable", "cache_unavailable"
		}
		recordMediaCache(ctx, name, mediaReason)
		recordDownloadCache(ctx, name, downloadReason)
		recordPreIDCache(ctx, name)
		if name != "bypass" {
			// Real capacity removals produce cumulative snapshots 1/2, then
			// 3/4; repeated observations must not become an eviction delta.
			for entry := index * 2; entry < index*2+2; entry++ {
				key := "contract-media-" + strconv.Itoa(entry)
				media.put(key, RedirectCandidate{downloadCacheKey: "contract-download",
					resolvedSHA1: directPlaySourceSHA1, resolvedSize: 1}, cacheTime, cacheTime, cacheTime.Add(time.Hour), "contract-session")
			}
			start, end := index*2+1, index*2+3
			if index == 0 {
				start = 0
			}
			for entry := start; entry < end; entry++ {
				download.put("contract-download-"+strconv.Itoa(entry), p115.DownloadURLResult{
					URL: "https://cdn.115.com/contract-fixture", ExpiresAt: cacheTime.Add(time.Hour),
				}, cacheTime)
			}
			recordCacheUse(ctx, media, download)
		}
		timing := recorder.finish()
		fields := cacheDiagnosticFields(t, timing)
		if fields["preIDWaitCalls"] != "2" || fields["preIDWaitMs"] == "0" {
			t.Fatal("fixture must exercise measured preID waiting")
		}
		for _, stage := range timingStages {
			if fields[stage+"Ms"] == "" || fields[stage+"Calls"] == "" {
				t.Fatalf("fixture omitted timing stage %s", stage)
			}
		}
		packet.Cases = append(packet.Cases, timingCase{Name: name, Fields: timing.LogFields()})
	}
	encoded, err := json.Marshal(packet)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("GATEWAY_DIAGNOSTIC_CONTRACT %s", encoded)
}
