package config

import (
	"errors"
	"testing"
)

func TestNormalizeRegistrationEmailDomains(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "empty input returns empty",
			input: "",
			want:  "",
		},
		{
			name:  "whitespace only returns empty",
			input: "   \n\t\n",
			want:  "",
		},
		{
			name:  "lowercases dedupes and sorts",
			input: "Gmail.com\noutlook.com\ngmail.com\n \noutlook.com\n",
			want:  "gmail.com\noutlook.com",
		},
		{
			name:  "trims per-line whitespace and ignores blank lines",
			input: "  gmail.com  \n\n   outlook.com\n",
			want:  "gmail.com\noutlook.com",
		},
		{
			name:  "preserves single domain",
			input: "Mail.Example.COM",
			want:  "mail.example.com",
		},
		{
			name:    "rejects scheme",
			input:   "https://gmail.com",
			wantErr: true,
		},
		{
			name:    "rejects port",
			input:   "gmail.com:465",
			wantErr: true,
		},
		{
			name:    "rejects path",
			input:   "gmail.com/login",
			wantErr: true,
		},
		{
			name:    "rejects wildcard",
			input:   "*.gmail.com",
			wantErr: true,
		},
		{
			name:    "rejects explicit at sign",
			input:   "user@gmail.com",
			wantErr: true,
		},
		{
			name:    "rejects single label",
			input:   "localhost",
			wantErr: true,
		},
		{
			name:    "rejects leading dot",
			input:   ".gmail.com",
			wantErr: true,
		},
		{
			name:    "rejects trailing dot",
			input:   "gmail.com.",
			wantErr: true,
		},
		{
			name:    "rejects empty label",
			input:   "gmail..com",
			wantErr: true,
		},
		{
			name:    "rejects label hyphen edges",
			input:   "-gmail.com",
			wantErr: true,
		},
		{
			name:    "rejects illegal chars",
			input:   "gma il.com",
			wantErr: true,
		},
		{
			name:    "rejects illegal symbol",
			input:   "gmail.c$m",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeRegistrationEmailDomains(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestRegistrationEmailDomainsConfigDefinition(t *testing.T) {
	def, ok := getConfigDefinitionMap()["registration_allowed_email_domains"]
	if !ok {
		t.Fatal("expected registration_allowed_email_domains definition to exist")
	}
	if def.Group != ConfigGroupBusiness {
		t.Fatalf("expected business group, got %s", def.Group)
	}
	if !def.Editable {
		t.Fatal("expected definition to be editable")
	}
	if !def.Multiline {
		t.Fatal("expected definition to be multiline")
	}
	if !def.AllowEmpty {
		t.Fatal("expected definition to allow empty value")
	}
	if def.EmptyValueMode != ConfigEmptyValueDisable {
		t.Fatalf("expected empty value mode disable, got %s", def.EmptyValueMode)
	}
	if def.Validate == nil {
		t.Fatal("expected validate function")
	}
	if def.Normalize == nil {
		t.Fatal("expected normalize function")
	}
	if def.Type != ConfigValueString {
		t.Fatalf("expected type string, got %s", def.Type)
	}
	if def.Sensitive {
		t.Fatal("expected definition not to be sensitive")
	}

	if err := def.Validate(""); err != nil {
		t.Fatalf("expected empty value to validate, got %v", err)
	}
	got, err := def.Normalize("Gmail.com\noutlook.com\ngmail.com\n")
	if err != nil {
		t.Fatalf("expected normalize success, got %v", err)
	}
	if got != "gmail.com\noutlook.com" {
		t.Fatalf("expected normalize to dedupe and sort, got %q", got)
	}

	if _, err := def.Normalize("https://gmail.com"); err == nil {
		t.Fatal("expected normalize to reject scheme")
	}
	if err := def.Validate("user@gmail.com"); err == nil {
		t.Fatal("expected validate to reject explicit @")
	}
	if err := def.Validate("*.gmail.com"); err == nil {
		t.Fatal("expected validate to reject wildcard")
	}
}

func TestIsRegistrationEmailAllowedEmptyAllowlistAllowsAll(t *testing.T) {
	service := &ConfigService{}
	// 空白名单等价于功能关闭，不依赖数据库 / 环境变量。
	if err := service.IsRegistrationEmailAllowed("anyone@example.com"); err != nil {
		t.Fatalf("expected empty allowlist to allow any email, got %v", err)
	}
}

func TestMatchRegistrationEmailAgainstDomains(t *testing.T) {
	domains, err := splitRegistrationEmailDomains("gmail.com\noutlook.com")
	if err != nil {
		t.Fatalf("setup splitRegistrationEmailDomains failed: %v", err)
	}

	tests := []struct {
		name    string
		email   string
		wantErr error
	}{
		{
			name:  "lowercase exact match",
			email: "user@gmail.com",
		},
		{
			name:  "uppercase domain matches",
			email: "User@GMAIL.COM",
		},
		{
			name:  "display name parsed correctly",
			email: "Display Name <user@gmail.com>",
		},
		{
			name:    "subdomain not matched",
			email:   "a@mail.gmail.com",
			wantErr: ErrRegistrationEmailDomainNotAllowed,
		},
		{
			name:    "different domain rejected",
			email:   "user@yahoo.com",
			wantErr: ErrRegistrationEmailDomainNotAllowed,
		},
		{
			name:    "invalid email rejected",
			email:   "not-an-email",
			wantErr: ErrRegistrationEmailInvalid,
		},
		{
			name:    "empty email rejected",
			email:   "   ",
			wantErr: ErrRegistrationEmailInvalid,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := matchRegistrationEmailAgainstDomains(tc.email, domains)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestMatchRegistrationEmailAgainstEmptyAllowlist(t *testing.T) {
	if err := matchRegistrationEmailAgainstDomains("user@anywhere.com", nil); err != nil {
		t.Fatalf("expected nil allowlist to allow any email, got %v", err)
	}
}
