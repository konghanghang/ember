package email

import (
	"errors"
	"testing"

	"github.com/konghang/ember/backend/internal/models"
)

type stubEmailConfigReader struct {
	values            map[string]string
	verificationOn    bool
	allowEmailErrFn   func(email string) error
	allowEmailCalled  bool
	allowEmailLastArg string
}

func (s *stubEmailConfigReader) GetString(key string) string {
	if s == nil || s.values == nil {
		return ""
	}
	return s.values[key]
}

func (s *stubEmailConfigReader) IsEmailVerificationEnabled() bool {
	return s.verificationOn
}

func (s *stubEmailConfigReader) IsRegistrationEmailAllowed(email string) error {
	s.allowEmailCalled = true
	s.allowEmailLastArg = email
	if s.allowEmailErrFn == nil {
		return nil
	}
	return s.allowEmailErrFn(email)
}

func newConfiguredStubReader(allowErr error) *stubEmailConfigReader {
	return &stubEmailConfigReader{
		values: map[string]string{
			"SMTP_HOST":                 "smtp.example.com",
			"SMTP_PORT":                 "587",
			"SMTP_USERNAME":             "ember",
			"SMTP_PASSWORD":             "secret",
			"SMTP_FROM":                 "Ember <ember@example.com>",
			"EMAIL_CODE_EXPIRY_MINUTES": "10",
			"EMAIL_CODE_DAILY_LIMIT":    "5",
			"EMAIL_CODE_IP_DAILY_LIMIT": "15",
		},
		verificationOn: true,
		allowEmailErrFn: func(string) error {
			return allowErr
		},
	}
}

func TestSendVerificationCodeRejectsRegistrationDomainBeforeAnyDBWork(t *testing.T) {
	reader := newConfiguredStubReader(errors.New("domain blocked"))
	service := NewEmailServiceWithConfig(reader)

	err := service.SendVerificationCode("user@yahoo.com", "127.0.0.1", models.VerificationTypeRegister)
	if !errors.Is(err, ErrEmailDomainNotAllowed) {
		t.Fatalf("expected ErrEmailDomainNotAllowed, got %v", err)
	}
	if !reader.allowEmailCalled {
		t.Fatalf("expected allowlist hook to be consulted")
	}
	if reader.allowEmailLastArg != "user@yahoo.com" {
		t.Fatalf("expected hook to receive original email, got %q", reader.allowEmailLastArg)
	}
}

func TestSendVerificationCodeRegisterPassesAllowlistDomain(t *testing.T) {
	// 域名校验通过时不能误报 ErrEmailDomainNotAllowed；下游因没有真实 db
	// 会进入其他失败路径，本测试只断言不会卡在域名拦截。
	reader := newConfiguredStubReader(nil)
	service := NewEmailServiceWithConfig(reader)

	defer func() {
		// 下游 db.DB.Model(...) 在 db.DB == nil 时会 panic；
		// 我们关心的断言已经在 panic 前完成（域名校验不阻断 + hook 被调用）。
		_ = recover()
		if !reader.allowEmailCalled {
			t.Fatalf("expected allowlist hook to be consulted in register path")
		}
	}()

	err := service.SendVerificationCode("user@gmail.com", "127.0.0.1", models.VerificationTypeRegister)
	if errors.Is(err, ErrEmailDomainNotAllowed) {
		t.Fatalf("expected allowlist allow path not to return ErrEmailDomainNotAllowed, got %v", err)
	}
}

func TestSendVerificationCodeResetSkipsAllowlistHook(t *testing.T) {
	// 反账号枚举设计要求 reset 路径不做域名门控，且不消费 allowlist hook。
	// 下游 db 调用会 panic，这里通过 recover 捕获后断言 hook 未被消费。
	reader := newConfiguredStubReader(errors.New("domain blocked"))
	service := NewEmailServiceWithConfig(reader)

	defer func() {
		_ = recover()
		if reader.allowEmailCalled {
			t.Fatalf("expected reset path to skip the domain allowlist hook entirely")
		}
	}()

	_ = service.SendVerificationCode("user@yahoo.com", "127.0.0.1", models.VerificationTypeReset)
}
