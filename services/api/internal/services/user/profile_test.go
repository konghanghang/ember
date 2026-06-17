package user

import (
	"errors"
	"testing"

	"github.com/konghang/ember/backend/internal/models"
	emailpkg "github.com/konghang/ember/backend/internal/services/email"
	"gorm.io/gorm"
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

func TestHashEmailNormalizesAndRedacts(t *testing.T) {
	testCases := []struct {
		name  string
		email string
		want  string
	}{
		{name: "blank", email: "  ", want: "empty"},
		{name: "normalizes case and whitespace", email: " Ember@Example.COM ", want: "1cb2bc5c"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hashEmail(tc.email); got != tc.want {
				t.Fatalf("expected hash %q, got %q", tc.want, got)
			}
		})
	}
}

func TestUpdateEmailUsesInjectedPersistenceAndReturnsOldEmail(t *testing.T) {
	verifier := &stubUserEmailVerifier{}
	service := &UserService{
		emailVerifier: verifier,
		findUserByID: func(userID string) (*models.User, error) {
			if userID != "user_1" {
				t.Fatalf("unexpected user id: %s", userID)
			}
			return &models.User{ID: userID, Email: "old@example.com"}, nil
		},
		updateEmailWithCode: func(userID, newEmail, code string) error {
			if userID != "user_1" || newEmail != "new@example.com" || code != "123456" {
				t.Fatalf("unexpected update payload: userID=%s newEmail=%s code=%s", userID, newEmail, code)
			}
			return nil
		},
	}

	result, err := service.UpdateEmail("user_1", &UpdateEmailRequest{
		NewEmail: " new@example.com ",
		Code:     "123456",
	})

	if err != nil {
		t.Fatalf("expected update success, got %v", err)
	}
	if verifier.domainCheckLastEmail != "new@example.com" {
		t.Fatalf("expected allowlist check for trimmed new email, got %q", verifier.domainCheckLastEmail)
	}
	if result == nil || result.OldEmail != "old@example.com" || result.User == nil || result.User.Email != "new@example.com" {
		t.Fatalf("unexpected update result: %+v", result)
	}
}

func TestUpdateEmailRejectsUnchangedEmailBeforeAllowlistAndPersistence(t *testing.T) {
	verifier := &stubUserEmailVerifier{}
	service := &UserService{
		emailVerifier: verifier,
		findUserByID: func(userID string) (*models.User, error) {
			return &models.User{ID: userID, Email: "old@example.com"}, nil
		},
		updateEmailWithCode: func(userID, newEmail, code string) error {
			t.Fatalf("updateEmailWithCode must not run for unchanged email")
			return nil
		},
	}

	result, err := service.UpdateEmail("user_1", &UpdateEmailRequest{
		NewEmail: " OLD@example.com ",
		Code:     "123456",
	})

	if !errors.Is(err, emailpkg.ErrEmailUnchanged) {
		t.Fatalf("expected ErrEmailUnchanged, got %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result on failure, got %+v", result)
	}
	if verifier.domainCheckCalled {
		t.Fatalf("allowlist check must not run for unchanged email")
	}
}

func TestUpdateEmailStopsWhenDomainRejectedBeforePersistence(t *testing.T) {
	verifier := &stubUserEmailVerifier{domainErr: errors.New("domain blocked")}
	service := &UserService{
		emailVerifier: verifier,
		findUserByID: func(userID string) (*models.User, error) {
			return &models.User{ID: userID, Email: "old@example.com"}, nil
		},
		updateEmailWithCode: func(userID, newEmail, code string) error {
			t.Fatalf("updateEmailWithCode must not run when allowlist rejects")
			return nil
		},
	}

	result, err := service.UpdateEmail("user_1", &UpdateEmailRequest{
		NewEmail: "new@example.com",
		Code:     "123456",
	})

	if err == nil || err.Error() != "domain blocked" {
		t.Fatalf("expected domain rejection, got %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result on failure, got %+v", result)
	}
}

func TestUpdateEmailMapsMissingUserBeforeAllowlistAndPersistence(t *testing.T) {
	verifier := &stubUserEmailVerifier{}
	service := &UserService{
		emailVerifier: verifier,
		findUserByID: func(userID string) (*models.User, error) {
			return nil, gorm.ErrRecordNotFound
		},
		updateEmailWithCode: func(userID, newEmail, code string) error {
			t.Fatalf("updateEmailWithCode must not run when user is missing")
			return nil
		},
	}

	result, err := service.UpdateEmail("missing_user", &UpdateEmailRequest{
		NewEmail: "new@example.com",
		Code:     "123456",
	})

	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result on failure, got %+v", result)
	}
	if verifier.domainCheckCalled {
		t.Fatalf("allowlist check must not run when user lookup fails")
	}
}
