package directplay

import (
	"context"
	"strings"
	"testing"
	"time"

	p115 "github.com/konghang/ember/backend/internal/integrations/p115"
	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/services/p115account"
)

// TestPreIDKeyIsolation binds all source identity and exact Provider inputs while
// excluding target/caller fields that do not affect the immutable source prefix.
func TestPreIDKeyIsolation(t *testing.T) {
	source, _ := (fakeAccountLoader{}).LoadActiveCredentialByRole(context.Background(), models.P115AccountRoleSource)
	file := newFakeProvider().sourceFile
	byteRange := boundedRange(file.Size)
	key := preIDKey(source, file, byteRange)
	if len(key) != 64 || strings.Contains(key, source.Credential.Cookie) {
		t.Fatal("preID identity must be an opaque SHA256 digest")
	}
	mutations := []struct {
		name   string
		mutate func(*p115account.ActiveAccountCredential, *p115.File, *p115.ByteRange)
	}{
		{"account", func(a *p115account.ActiveAccountCredential, _ *p115.File, _ *p115.ByteRange) {
			a.Credential.AccountID += "2"
		}},
		{"provider UID", func(a *p115account.ActiveAccountCredential, _ *p115.File, _ *p115.ByteRange) { a.ProviderUserID += "2" }},
		{"version", func(a *p115account.ActiveAccountCredential, _ *p115.File, _ *p115.ByteRange) {
			a.DownloadCacheVersion++
		}},
		{"exact credential", func(a *p115account.ActiveAccountCredential, _ *p115.File, _ *p115.ByteRange) {
			a.Credential.Cookie += " "
		}},
		{"app type", func(a *p115account.ActiveAccountCredential, _ *p115.File, _ *p115.ByteRange) {
			a.Credential.AppType = "web"
		}},
		{"source UA", func(a *p115account.ActiveAccountCredential, _ *p115.File, _ *p115.ByteRange) {
			a.Credential.UserAgent += "2"
		}},
		{"file ID", func(_ *p115account.ActiveAccountCredential, f *p115.File, _ *p115.ByteRange) { f.ID += "2" }},
		{"pick code", func(_ *p115account.ActiveAccountCredential, f *p115.File, _ *p115.ByteRange) { f.PickCode += "2" }},
		{"full SHA1", func(_ *p115account.ActiveAccountCredential, f *p115.File, _ *p115.ByteRange) {
			f.SHA1 = strings.Repeat("B", 40)
		}},
		{"size", func(_ *p115account.ActiveAccountCredential, f *p115.File, _ *p115.ByteRange) { f.Size++ }},
		{"range", func(_ *p115account.ActiveAccountCredential, _ *p115.File, r *p115.ByteRange) { r.End-- }},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			account, changed, r := source, file, byteRange
			test.mutate(&account, &changed, &r)
			if other := preIDKey(account, changed, r); other == "" || other == key {
				t.Fatal("changed source identity reused the same cache key")
			}
		})
	}
	// A length-prefixed encoding must not collide across field boundaries.
	left, right := source, source
	left.Credential.AccountID, left.ProviderUserID = "ab", "c"
	right.Credential.AccountID, right.ProviderUserID = "a", "bc"
	if preIDKey(left, file, byteRange) == preIDKey(right, file, byteRange) {
		t.Fatal("ambiguous field concatenation")
	}
	source.TargetParentID, source.EmbyPathPrefix, source.SourceRootID = "other-target", "/other", "99"
	file.Name, file.ParentID = "renamed.mkv", "99"
	if preIDKey(source, file, byteRange) != key {
		t.Fatal("path/target metadata changed an identical source identity")
	}
}

// TestPreIDKeyIncompleteIdentityBypasses keeps uncertain snapshots out of both
// the cache and flight map, especially active account recovery version zero.
func TestPreIDKeyIncompleteIdentityBypasses(t *testing.T) {
	source, _ := (fakeAccountLoader{}).LoadActiveCredentialByRole(context.Background(), models.P115AccountRoleSource)
	file := newFakeProvider().sourceFile
	tests := []struct {
		name   string
		mutate func(*p115account.ActiveAccountCredential, *p115.File)
	}{
		{"zero version", func(a *p115account.ActiveAccountCredential, _ *p115.File) { a.DownloadCacheVersion = 0 }},
		{"negative version", func(a *p115account.ActiveAccountCredential, _ *p115.File) { a.DownloadCacheVersion = -1 }},
		{"account", func(a *p115account.ActiveAccountCredential, _ *p115.File) { a.Credential.AccountID = "" }},
		{"UID", func(a *p115account.ActiveAccountCredential, _ *p115.File) { a.ProviderUserID = "" }},
		{"credential", func(a *p115account.ActiveAccountCredential, _ *p115.File) { a.Credential.Cookie = " " }},
		{"app type", func(a *p115account.ActiveAccountCredential, _ *p115.File) { a.Credential.AppType = "" }},
		{"source UA", func(a *p115account.ActiveAccountCredential, _ *p115.File) { a.Credential.UserAgent = "" }},
		{"file ID", func(_ *p115account.ActiveAccountCredential, f *p115.File) { f.ID = "" }},
		{"pick code", func(_ *p115account.ActiveAccountCredential, f *p115.File) { f.PickCode = "" }},
		{"SHA1", func(_ *p115account.ActiveAccountCredential, f *p115.File) { f.SHA1 = "invalid" }},
		{"size", func(_ *p115account.ActiveAccountCredential, f *p115.File) { f.Size = 0 }},
		{"directory", func(_ *p115account.ActiveAccountCredential, f *p115.File) { f.IsDirectory = true }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			account, changed := source, file
			test.mutate(&account, &changed)
			if preIDKey(account, changed, boundedRange(changed.Size)) != "" {
				t.Fatal("incomplete source identity enabled cache reuse")
			}
		})
	}
}

// TestPreIDCacheFixedTTLAndCapacity verifies non-sliding deadlines, access LRU,
// lazy expiry and rollback deletion without a background clock or worker.
func TestPreIDCacheFixedTTLAndCapacity(t *testing.T) {
	if preIDCacheCapacity != 1024 || preIDCacheTTL != 10*time.Minute {
		t.Fatal("preID cache changed the fixed capacity/TTL contract")
	}
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.FixedZone("fixture", 8*60*60))
	byteRange := boundedRange(17)
	result := preIDHash(p115.FileRangeRequest{Range: byteRange})
	cache := newPreIDCache(2)
	cache.put(context.Background(), "a", result, byteRange, now)
	cache.put(context.Background(), "b", result, byteRange, now)
	if got, found := cache.get("a", now.Add(preIDCacheTTL-time.Nanosecond)); !found || got != result {
		t.Fatal("valid short-file prefix was not retained until its deadline")
	}
	cache.put(context.Background(), "c", result, byteRange, now.Add(time.Second))
	if len(cache.entries) != 2 {
		t.Fatalf("capacity=%d, want 2", len(cache.entries))
	}
	if _, found := cache.get("b", now.Add(time.Second)); found {
		t.Fatal("least recently used entry was not evicted")
	}
	if _, found := cache.get("a", now.Add(preIDCacheTTL)); found {
		t.Fatal("access extended the fixed TTL")
	}
	if _, found := cache.get("c", now); found || len(cache.entries) != 0 {
		t.Fatal("rollback did not remove the future entry")
	}
	for i := range preIDCacheCapacity + 20 {
		cache.put(context.Background(), string(rune(i+1)), result, byteRange, now)
	}
	if len(cache.entries) != 2 {
		t.Fatal("repeated publication exceeded bounded capacity")
	}
}

// TestPreIDCacheRejectsInvalidAndCanceledPublication protects the successful-only
// cache boundary even if a future caller bypasses the usual read validation.
func TestPreIDCacheRejectsInvalidAndCanceledPublication(t *testing.T) {
	range17 := boundedRange(17)
	valid := preIDHash(p115.FileRangeRequest{Range: range17})
	for _, test := range []struct {
		name     string
		result   p115.FileRangeHash
		canceled bool
	}{
		{name: "invalid SHA1", result: p115.FileRangeHash{SHA1: "invalid", BytesRead: 17}},
		{name: "short read", result: p115.FileRangeHash{SHA1: valid.SHA1, BytesRead: 16}},
		{name: "long read", result: p115.FileRangeHash{SHA1: valid.SHA1, BytesRead: 18}},
		{name: "canceled", result: valid, canceled: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			cache := newPreIDCache(1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if test.canceled {
				cancel()
			}
			cache.put(ctx, "fixture", test.result, range17, time.Now())
			if len(cache.entries) != 0 {
				t.Fatal("unsuccessful source read was cached")
			}
		})
	}
}
