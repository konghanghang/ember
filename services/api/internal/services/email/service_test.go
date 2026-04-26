package email

import "testing"

func TestEmailServiceHasSMTPConfig(t *testing.T) {
	tests := []struct {
		name    string
		service EmailService
		want    bool
	}{
		{
			name: "missing host",
			service: EmailService{
				username:    "user",
				password:    "secret",
				fromAddress: "ember@example.com",
			},
			want: false,
		},
		{
			name: "missing username",
			service: EmailService{
				host:        "smtp.example.com",
				password:    "secret",
				fromAddress: "ember@example.com",
			},
			want: false,
		},
		{
			name: "missing password",
			service: EmailService{
				host:        "smtp.example.com",
				username:    "user",
				fromAddress: "ember@example.com",
			},
			want: false,
		},
		{
			name: "missing from address",
			service: EmailService{
				host:     "smtp.example.com",
				username: "user",
				password: "secret",
			},
			want: false,
		},
		{
			name: "fully configured",
			service: EmailService{
				host:        "smtp.example.com",
				username:    "user",
				password:    "secret",
				fromAddress: "ember@example.com",
			},
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := tc.service
			if got := svc.hasSMTPConfig(); got != tc.want {
				t.Fatalf("expected %t, got %t", tc.want, got)
			}
		})
	}
}

func TestValidateVerificationRateLimits(t *testing.T) {
	tests := []struct {
		name         string
		emailCount   int64
		ipCount      int64
		dailyLimit   int
		ipDailyLimit int
		wantErr      error
	}{
		{
			name:         "email daily limit reached",
			emailCount:   5,
			ipCount:      0,
			dailyLimit:   5,
			ipDailyLimit: 15,
			wantErr:      ErrEmailCodeRateLimit,
		},
		{
			name:         "ip daily limit reached",
			emailCount:   1,
			ipCount:      15,
			dailyLimit:   5,
			ipDailyLimit: 15,
			wantErr:      ErrEmailCodeIPRateLimit,
		},
		{
			name:         "below limits",
			emailCount:   2,
			ipCount:      3,
			dailyLimit:   5,
			ipDailyLimit: 15,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateVerificationRateLimits(tc.emailCount, tc.ipCount, tc.dailyLimit, tc.ipDailyLimit)
			if tc.wantErr == nil && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr != nil && err != tc.wantErr {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestNormalizeVerificationEmail(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  string
	}{
		{
			name:  "lowercases and trims",
			email: "  Ember.User@Example.COM  ",
			want:  "ember.user@example.com",
		},
		{
			name:  "empty remains empty",
			email: "   ",
			want:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeVerificationEmail(tc.email); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}
