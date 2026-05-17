package common

import "testing"

func TestValidateInternalAPISecret(t *testing.T) {
	tests := []struct {
		name    string
		secret  string
		wantErr bool
	}{
		{name: "empty", secret: "", wantErr: true},
		{name: "placeholder", secret: "your-internal-api-secret", wantErr: true},
		{name: "short", secret: "short-secret", wantErr: true},
		{name: "valid", secret: "0123456789abcdef0123456789abcdef", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInternalAPISecret(tt.secret)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateInternalAPISecret() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
