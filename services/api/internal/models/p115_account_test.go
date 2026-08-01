package models

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestP115AccountUsesExplicitColumnMappings(t *testing.T) {
	modelType := reflect.TypeOf(P115Account{})
	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		if !strings.Contains(field.Tag.Get("gorm"), "column:") {
			t.Fatalf("P115Account.%s must declare an explicit gorm column", field.Name)
		}
	}
}

func TestP115AccountNeverSerializesCredentialCiphertext(t *testing.T) {
	account := P115Account{
		ID:               "account_1",
		CookieCiphertext: "encrypted-cookie-value",
	}

	payload, err := json.Marshal(account)
	if err != nil {
		t.Fatalf("json.Marshal() failed: %v", err)
	}
	if strings.Contains(string(payload), account.CookieCiphertext) || strings.Contains(string(payload), "cookieCiphertext") {
		t.Fatalf("serialized account exposed credential ciphertext: %s", payload)
	}
}

func TestP115AccountBeforeCreateAppliesSafeDefaults(t *testing.T) {
	account := &P115Account{}
	callBeforeCreate(t, account)

	if account.AuthMode != P115AuthModeLegacyCookie {
		t.Fatalf("AuthMode = %q, want legacy_cookie", account.AuthMode)
	}
	if account.Status != P115AccountStatusPending {
		t.Fatalf("Status = %q, want pending", account.Status)
	}
	if account.Enabled {
		t.Fatal("new account must remain disabled until validation")
	}
}
