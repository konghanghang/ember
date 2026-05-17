package common

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestParseTokenRejectsNonHS256SigningMethod(t *testing.T) {
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	if err := InitJWT(); err != nil {
		t.Fatalf("InitJWT() error = %v", err)
	}

	claims := Claims{
		UserID:   "user_1",
		Username: "alice",
		Role:     "user",
		PwdSig:   "sig",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "ember-api",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS384, claims)
	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}

	if _, err := ParseToken(tokenString); err == nil {
		t.Fatalf("expected HS384 token to be rejected by valid methods, got %v", err)
	}
}
