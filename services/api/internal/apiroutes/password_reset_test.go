package apiroutes

import "testing"

func TestPasswordResetClosedLoopPaths(t *testing.T) {
	want := []string{
		"/api/v1/profile",
		"/api/v1/password",
		"/api/v1/account-links",
		"/api/v1/admin/current",
		"/api/v1/user/profile",
		"/api/v1/user/password",
	}

	got := PasswordResetClosedLoopPaths()
	if len(got) != len(want) {
		t.Fatalf("expected %d paths, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("path[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestPasswordResetClosedLoopPathsReturnsCopy(t *testing.T) {
	paths := PasswordResetClosedLoopPaths()
	paths[0] = "/polluted"

	fresh := PasswordResetClosedLoopPaths()
	if fresh[0] == "/polluted" {
		t.Fatalf("PasswordResetClosedLoopPaths returned shared backing array")
	}
	if fresh[0] != FullProfilePath {
		t.Fatalf("expected first path %q, got %q", FullProfilePath, fresh[0])
	}
}
