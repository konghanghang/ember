package user

import (
	"errors"
	"testing"

	emailpkg "github.com/konghang/ember/backend/internal/services/email"
)

// TestUnchangedEmailCheck 覆盖 UpdateEmail 入口的“不变化”判定：
// 大小写不敏感 + 去首尾空白命中即返回 ErrEmailUnchanged，不进入事务也不消耗验证码配额。
//
// 真实 UpdateEmail 在加载 user 后调用同一份判定函数；这条测试只锚定判定语义，
// 其他事务路径（验证码错误、唯一冲突、成功落库）依赖真实 PostgreSQL，由集成测试覆盖。
func TestUnchangedEmailCheck(t *testing.T) {
	cases := []struct {
		name     string
		current  string
		next     string
		wantSame bool
	}{
		{name: "exact match", current: "ember@example.com", next: "ember@example.com", wantSame: true},
		{name: "case insensitive", current: "ember@example.com", next: "Ember@Example.COM", wantSame: true},
		{name: "trims whitespace", current: "ember@example.com", next: "  ember@example.com  ", wantSame: true},
		{name: "different mailbox", current: "ember@example.com", next: "new@example.com", wantSame: false},
		{name: "different domain", current: "ember@example.com", next: "ember@example.org", wantSame: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := unchangedEmailCheck(tc.current, tc.next)
			if tc.wantSame {
				if !errors.Is(err, emailpkg.ErrEmailUnchanged) {
					t.Fatalf("expected ErrEmailUnchanged, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}
