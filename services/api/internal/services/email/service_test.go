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

func TestValidateVerificationRecipient(t *testing.T) {
	tests := []struct {
		name              string
		codeType          string
		existingUserCount int64
		wantErr           error
	}{
		{
			name:              "register rejects existing email",
			codeType:          "register",
			existingUserCount: 1,
			wantErr:           ErrEmailAlreadyRegistered,
		},
		{
			name:              "reset rejects missing email",
			codeType:          "reset",
			existingUserCount: 0,
			wantErr:           ErrEmailNotRegistered,
		},
		{
			name:              "register allows new email",
			codeType:          "register",
			existingUserCount: 0,
		},
		{
			name:              "reset allows existing email",
			codeType:          "reset",
			existingUserCount: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateVerificationRecipient(tc.codeType, tc.existingUserCount)
			if tc.wantErr == nil && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr != nil && err != tc.wantErr {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
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
