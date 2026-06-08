package auth

import (
	"errors"
	"testing"
	"time"

	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	notifierint "github.com/konghang/ember/backend/internal/integrations/notifier"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

type stubAuthEmailVerifier struct {
	enabled       bool
	lastEmail     string
	lastCode      string
	lastCodeType  string
	checkCalled   bool
	verifyCodeErr error
}

func (s *stubAuthEmailVerifier) IsEnabled() bool {
	return s.enabled
}

func (s *stubAuthEmailVerifier) CheckCode(email, code, codeType string) error {
	s.checkCalled = true
	s.lastEmail = email
	s.lastCode = code
	s.lastCodeType = codeType
	return s.verifyCodeErr
}

func (s *stubAuthEmailVerifier) ConsumeCodeTx(_ *gorm.DB, email, code, codeType string) error {
	s.lastEmail = email
	s.lastCode = code
	s.lastCodeType = codeType
	return s.verifyCodeErr
}

type stubAuthRegistrationNotifier struct {
	configured bool
	calledCh   chan notifierint.RegistrationNotification
}

func (s *stubAuthRegistrationNotifier) IsConfigured() bool {
	return s.configured
}

func (s *stubAuthRegistrationNotifier) NotifyNewRegistration(data notifierint.RegistrationNotification) {
	if s.calledCh != nil {
		s.calledCh <- data
	}
}

func TestValidateRegisterRequest(t *testing.T) {
	service := &AuthService{}

	tests := []struct {
		name     string
		req      RegisterUserRequest
		wantErr  string
		wantUser string
	}{
		{
			name:     "trimmed username valid",
			req:      RegisterUserRequest{Username: "  ember123  "},
			wantUser: "ember123",
		},
		{
			name:    "username too short",
			req:     RegisterUserRequest{Username: "ab"},
			wantErr: "用户名长度必须为 3-50 位",
		},
		{
			name:    "username invalid chars",
			req:     RegisterUserRequest{Username: "ember-123"},
			wantErr: "用户名只能包含字母和数字",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := tc.req
			err := service.validateRegisterRequest(&req)
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("expected error %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if req.Username != tc.wantUser {
				t.Fatalf("expected trimmed username %q, got %q", tc.wantUser, req.Username)
			}
		})
	}
}

func TestVerifyRegisterEmailCode(t *testing.T) {
	t.Run("disabled email verification skips validation", func(t *testing.T) {
		emailVerifier := &stubAuthEmailVerifier{enabled: false}
		service := &AuthService{emailService: emailVerifier}

		err := service.verifyRegisterEmailCode(&RegisterUserRequest{
			Email:     "ember@example.com",
			EmailCode: "",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if emailVerifier.checkCalled {
			t.Fatalf("expected verify not to be called when email verification is disabled")
		}
	})

	t.Run("enabled email verification requires code", func(t *testing.T) {
		emailVerifier := &stubAuthEmailVerifier{enabled: true}
		service := &AuthService{emailService: emailVerifier}

		err := service.verifyRegisterEmailCode(&RegisterUserRequest{
			Email: "ember@example.com",
		})
		if err == nil || err.Error() != "请先获取邮箱验证码" {
			t.Fatalf("expected missing code error, got %v", err)
		}
		if emailVerifier.checkCalled {
			t.Fatalf("expected verify not to be called when email code is missing")
		}
	})

	t.Run("enabled email verification delegates to verifier", func(t *testing.T) {
		emailVerifier := &stubAuthEmailVerifier{enabled: true}
		service := &AuthService{emailService: emailVerifier}

		err := service.verifyRegisterEmailCode(&RegisterUserRequest{
			Email:     "ember@example.com",
			EmailCode: "123456",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !emailVerifier.checkCalled {
			t.Fatalf("expected verify to be called")
		}
		if emailVerifier.lastEmail != "ember@example.com" || emailVerifier.lastCode != "123456" || emailVerifier.lastCodeType != models.VerificationTypeRegister {
			t.Fatalf("unexpected verify payload: email=%q code=%q type=%q", emailVerifier.lastEmail, emailVerifier.lastCode, emailVerifier.lastCodeType)
		}
	})

	t.Run("enabled email verification returns verifier error", func(t *testing.T) {
		expectedErr := errors.New("boom")
		emailVerifier := &stubAuthEmailVerifier{enabled: true, verifyCodeErr: expectedErr}
		service := &AuthService{emailService: emailVerifier}

		err := service.verifyRegisterEmailCode(&RegisterUserRequest{
			Email:     "ember@example.com",
			EmailCode: "123456",
		})
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected delegated error, got %v", err)
		}
	})
}

func TestEnforceRegisterEmailDomain(t *testing.T) {
	t.Run("nil hook treats as allowlist disabled", func(t *testing.T) {
		service := &AuthService{}
		if err := service.enforceRegisterEmailDomain(&RegisterUserRequest{Email: "user@anywhere.com"}); err != nil {
			t.Fatalf("expected nil hook to allow registration, got %v", err)
		}
	})

	t.Run("hook error transparently returned", func(t *testing.T) {
		expected := errors.New("domain rejected")
		service := &AuthService{
			isRegistrationEmailAllowed: func(email string) error {
				if email != "user@yahoo.com" {
					t.Fatalf("expected enforce to forward original email, got %q", email)
				}
				return expected
			},
		}

		err := service.enforceRegisterEmailDomain(&RegisterUserRequest{
			Username: "ember",
			Email:    "user@yahoo.com",
		})
		if !errors.Is(err, expected) {
			t.Fatalf("expected hook error to be returned, got %v", err)
		}
	})
}

func TestRegisterUserStopsWhenEmailDomainRejected(t *testing.T) {
	expected := errors.New("domain rejected")
	emailVerifier := &stubAuthEmailVerifier{enabled: true}
	embyClient := &stubAuthEmbyClient{}

	service := &AuthService{
		emailService: emailVerifier,
		isRegistrationEmailAllowed: func(string) error {
			return expected
		},
		newEmbyClient: func() authEmbyClient { return embyClient },
		// 故意不注入 saveUser / notifier / 兑换码校验等下游依赖；
		// 一旦域名拦截没生效，链路会立刻 panic 暴露问题。
	}

	_, err := service.RegisterUser(&RegisterUserRequest{
		Username:  "ember123",
		Password:  "secret123",
		Email:     "user@yahoo.com",
		EmailCode: "123456",
	})
	if !errors.Is(err, expected) {
		t.Fatalf("expected domain rejection, got %v", err)
	}
	if emailVerifier.checkCalled {
		t.Fatalf("expected email verifier not to be called when domain is rejected")
	}
}

func TestRegisterUserSkipsDomainCheckWhenHookNil(t *testing.T) {
	// 兜底：未注入 hook 时（例如老的 deps 默认值不可用），enforceRegisterEmailDomain
	// 必须等价于“白名单关闭”，不能默认拒绝所有人。链路在邮件码校验阶段终止以验证流程顺序。
	emailVerifier := &stubAuthEmailVerifier{enabled: true}
	service := &AuthService{
		emailService: emailVerifier,
		// isRegistrationEmailAllowed 故意留空
	}

	_, err := service.RegisterUser(&RegisterUserRequest{
		Username:  "ember123",
		Password:  "secret123",
		Email:     "user@anywhere.com",
		EmailCode: "",
	})
	if !errors.Is(err, ErrRegisterEmailCodeRequired) {
		t.Fatalf("expected to fall through to email code check, got %v", err)
	}
}

func TestNotifyNewRegistration(t *testing.T) {
	t.Run("skips when notifier is not configured", func(t *testing.T) {
		notifier := &stubAuthRegistrationNotifier{
			configured: false,
			calledCh:   make(chan notifierint.RegistrationNotification, 1),
		}
		service := &AuthService{notifier: notifier}
		service.notifyNewRegistration(models.User{Username: "ember"}, "open")

		select {
		case payload := <-notifier.calledCh:
			t.Fatalf("expected notifier not to be called, got payload %+v", payload)
		case <-time.After(50 * time.Millisecond):
		}
	})

	t.Run("configured notifier receives expected payload", func(t *testing.T) {
		notifier := &stubAuthRegistrationNotifier{
			configured: true,
			calledCh:   make(chan notifierint.RegistrationNotification, 1),
		}
		service := &AuthService{notifier: notifier}

		expiresAtTime := time.Date(2026, 3, 27, 10, 30, 0, 0, time.UTC)
		service.notifyNewRegistration(models.User{
			ID:        "user_1",
			Username:  "ember",
			Email:     "ember@example.com",
			EmbyID:    "emby_1",
			ExpiresAt: &expiresAtTime,
		}, "invite")

		select {
		case payload := <-notifier.calledCh:
			if payload.ID != "user_1" || payload.UserName != "ember" || payload.Email != "ember@example.com" || payload.EmbyID != "emby_1" || payload.RegistrationMode != "invite" {
				t.Fatalf("unexpected payload: %+v", payload)
			}
			if payload.ExpiresAt == nil || *payload.ExpiresAt != "2026-03-27 10:30:00 UTC" {
				t.Fatalf("unexpected expiresAt payload: %+v", payload.ExpiresAt)
			}
		case <-time.After(200 * time.Millisecond):
			t.Fatal("expected notifier to be called")
		}
	})
}

func TestPrepareRegisterInviteUsesRegistrationValidation(t *testing.T) {
	planGroup := "VIP_A"
	validateCalled := false
	service := &AuthService{
		getRegistrationMode: func() string { return "invite" },
		validateRegistrationCode: func(code string) (*models.RedemptionCode, error) {
			validateCalled = true
			if code != "invite-code" {
				t.Fatalf("unexpected code: %s", code)
			}
			return &models.RedemptionCode{
				ID:                    "rcode_1",
				Code:                  code,
				DefaultDays:           30,
				RegistrationPlanGroup: planGroup,
			}, nil
		},
	}

	prepared, err := service.prepareRegister(&RegisterUserRequest{Code: "invite-code"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !validateCalled {
		t.Fatalf("expected registration code validator to be called")
	}
	if prepared.defaultDays != 30 {
		t.Fatalf("expected defaultDays=30, got %d", prepared.defaultDays)
	}
	if prepared.registrationPlanGroup == nil || *prepared.registrationPlanGroup != "VIP_A" {
		t.Fatalf("expected registration plan group to be preserved, got %+v", prepared.registrationPlanGroup)
	}
}

func TestCreateEmbyUserForRegistrationInitialDisabled(t *testing.T) {
	tests := []struct {
		name                string
		defaultDays         int
		wantInitialDisabled bool
	}{
		{name: "zero day trial creates initially disabled emby user", defaultDays: 0, wantInitialDisabled: true},
		{name: "positive trial keeps emby user enabled initially", defaultDays: 7, wantInitialDisabled: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			embyClient := &stubAuthEmbyClient{
				createUserResp: &embyint.EmbyUser{ID: "emby_1"},
			}
			service := &AuthService{}

			user, err := service.createEmbyUserForRegistration(embyClient, &RegisterUserRequest{
				Username: "ember",
				Password: "secret123",
			}, &registerPreparation{
				defaultDays: tc.defaultDays,
			})
			if err != nil {
				t.Fatalf("expected success, got %v", err)
			}
			if user.ID != "emby_1" {
				t.Fatalf("expected emby user to be returned, got %+v", user)
			}
			if embyClient.lastCreateUser != "ember" || embyClient.lastCreatePwd != "secret123" {
				t.Fatalf("unexpected create payload: username=%q password=%q", embyClient.lastCreateUser, embyClient.lastCreatePwd)
			}
			if embyClient.lastInitialDisabled != tc.wantInitialDisabled {
				t.Fatalf("expected initialDisabled=%t, got %t", tc.wantInitialDisabled, embyClient.lastInitialDisabled)
			}
		})
	}
}

func TestBuildRegisteredUserAppliesRegistrationPlanGroup(t *testing.T) {
	planGroup := "VIP_A"
	service := &AuthService{}

	user, err := service.buildRegisteredUser(&RegisterUserRequest{
		Username: "ember",
		Password: "secret123",
		Email:    "ember@example.com",
	}, &registerPreparation{
		defaultDays:           30,
		registrationPlanGroup: &planGroup,
	}, &embyint.EmbyUser{ID: "emby_1"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if user.PlanGroup == nil || *user.PlanGroup != "VIP_A" {
		t.Fatalf("expected persisted user plan group VIP_A, got %+v", user.PlanGroup)
	}
}

func TestBuildRegisteredUserAppliesDefaultPlanGroup(t *testing.T) {
	service := &AuthService{
		getDefaultPlanGroup: func() (*models.PlanGroup, error) {
			return &models.PlanGroup{Key: "DEFAULT", Name: "默认分组"}, nil
		},
	}

	user, err := service.buildRegisteredUser(&RegisterUserRequest{
		Username: "ember",
		Password: "secret123",
		Email:    "ember@example.com",
	}, &registerPreparation{
		defaultDays: 30,
	}, &embyint.EmbyUser{ID: "emby_1"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if user.PlanGroup == nil || *user.PlanGroup != "DEFAULT" {
		t.Fatalf("expected persisted user plan group DEFAULT, got %+v", user.PlanGroup)
	}
}
