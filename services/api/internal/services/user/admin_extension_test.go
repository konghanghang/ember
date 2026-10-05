package user

import (
	"encoding/json"
	"errors"
	"testing"
)

// TestAdminEditExtensionRequiresSync ensures merged expiry edits still update the effective Emby policy.
func TestAdminEditExtensionRequiresSync(t *testing.T) {
	var req AdminUpdateUserRequest
	if err := json.Unmarshal([]byte(`{"extendDays":30,"operationId":"fixture"}`), &req); err != nil {
		t.Fatal(err)
	}
	if !adminUpdateChangesEmbyPolicy(&req) {
		t.Fatal("extension must require policy synchronization")
	}
}

// TestAdminEditExtensionValidation rejects ambiguous or non-idempotent mutations before accessing storage.
func TestAdminEditExtensionValidation(t *testing.T) {
	for _, body := range []string{
		`{"extendDays":0,"operationId":"fixture"}`,
		`{"extendDays":30}`,
		`{"extendDays":30,"operationId":"fixture","clearExpiresAt":true}`,
		`{"extendDays":30,"operationId":"fixture","planGroup":"OTHER"}`,
		`{"extendDays":30,"operationId":"fixture","expiresAt":"2099-01-01T00:00:00Z"}`,
	} {
		var req AdminUpdateUserRequest
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Fatal(err)
		}
		_, err := (&UserService{}).UpdateUserByAdmin("fixture", &req)
		if !errors.Is(err, ErrRequestInvalid) {
			t.Fatalf("body=%s error=%v", body, err)
		}
	}
}
