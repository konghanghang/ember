package email

import (
	"errors"
	"testing"
)

// 与 registration 路径共用 stubEmailConfigReader / newConfiguredStubReader（见 registration_domain_test.go），
// 这里只覆盖换邮箱链路特有的拦截顺序：
//   - unchanged 优先于 allowlist
//   - allowlist 拦截发生在任何 DB query 之前

func TestSendEmailChangeCodeRejectsBlockedDomainBeforeAnyDBWork(t *testing.T) {
	reader := newConfiguredStubReader(errors.New("domain blocked"))
	service := NewEmailServiceWithConfig(reader)

	err := service.SendEmailChangeCode("user@gmail.com", "user@yahoo.com", "127.0.0.1")
	if !errors.Is(err, ErrEmailDomainNotAllowed) {
		t.Fatalf("expected ErrEmailDomainNotAllowed, got %v", err)
	}
	if !reader.allowEmailCalled {
		t.Fatalf("expected allowlist hook to be consulted on change-email send-code path")
	}
	if reader.allowEmailLastArg != "user@yahoo.com" {
		t.Fatalf("expected hook to receive newEmail, got %q", reader.allowEmailLastArg)
	}
}

func TestSendEmailChangeCodeUnchangedEmailSkipsAllowlist(t *testing.T) {
	// 不变邮箱判定优先于域名门控；让用户在无变化时拿到精确文案，
	// 同时避免无意义地消费 allowlist 配置读取与潜在的 hook 副作用。
	reader := newConfiguredStubReader(errors.New("domain blocked"))
	service := NewEmailServiceWithConfig(reader)

	err := service.SendEmailChangeCode("user@gmail.com", "USER@gmail.com", "127.0.0.1")
	if !errors.Is(err, ErrEmailUnchanged) {
		t.Fatalf("expected ErrEmailUnchanged, got %v", err)
	}
	if reader.allowEmailCalled {
		t.Fatalf("expected unchanged-email check to short-circuit before allowlist hook")
	}
}

func TestSendEmailChangeCodePassesAllowlistDomain(t *testing.T) {
	// 允许域名时不能误返回 ErrEmailDomainNotAllowed；下游 db.DB.Model(...) 在 db.DB == nil 时
	// 会 panic，本测试只断言"允许路径不会被错误拦截"且 allowlist hook 已被消费。
	reader := newConfiguredStubReader(nil)
	service := NewEmailServiceWithConfig(reader)

	defer func() {
		_ = recover()
		if !reader.allowEmailCalled {
			t.Fatalf("expected allowlist hook to be consulted in change-email path")
		}
	}()

	err := service.SendEmailChangeCode("user@gmail.com", "user2@outlook.com", "127.0.0.1")
	if errors.Is(err, ErrEmailDomainNotAllowed) {
		t.Fatalf("expected allowlist allow path not to return ErrEmailDomainNotAllowed, got %v", err)
	}
}
