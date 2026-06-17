package common

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestInitJWTRequiresConfiguredStrongSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	if err := InitJWT(); err == nil || !strings.Contains(err.Error(), "未设置") {
		t.Fatalf("expected missing JWT_SECRET error, got %v", err)
	}

	t.Setenv("JWT_SECRET", "short-secret")
	if err := InitJWT(); err == nil || !strings.Contains(err.Error(), "至少 32 个字符") {
		t.Fatalf("expected short JWT_SECRET error, got %v", err)
	}

	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	if err := InitJWT(); err != nil {
		t.Fatalf("expected valid JWT_SECRET, got %v", err)
	}
}

func TestGenerateTokenParseTokenAndValidateToken(t *testing.T) {
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	if err := InitJWT(); err != nil {
		t.Fatalf("InitJWT() error = %v", err)
	}

	tokenString, err := GenerateToken("user_1", "alice", "user", "pwd_sig", 60)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	claims, err := ParseToken(tokenString)
	if err != nil {
		t.Fatalf("ParseToken() error = %v", err)
	}
	if claims.UserID != "user_1" || claims.Username != "alice" || claims.Role != "user" || claims.PwdSig != "pwd_sig" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if claims.Issuer != "ember-api" {
		t.Fatalf("expected issuer ember-api, got %q", claims.Issuer)
	}
	if claims.ExpiresAt == nil || claims.IssuedAt == nil || claims.NotBefore == nil {
		t.Fatalf("expected registered time claims to be populated: %+v", claims.RegisteredClaims)
	}
	if err := ValidateToken(tokenString); err != nil {
		t.Fatalf("ValidateToken() error = %v", err)
	}
}

func TestGenerateTokenUsesDefaultAndCustomExpiry(t *testing.T) {
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	if err := InitJWT(); err != nil {
		t.Fatalf("InitJWT() error = %v", err)
	}

	defaultToken, err := GenerateToken("user_1", "alice", "user", "sig")
	if err != nil {
		t.Fatalf("GenerateToken(default) error = %v", err)
	}
	defaultClaims, err := ParseToken(defaultToken)
	if err != nil {
		t.Fatalf("ParseToken(default) error = %v", err)
	}
	defaultTTL := defaultClaims.ExpiresAt.Time.Sub(defaultClaims.IssuedAt.Time)
	if defaultTTL < 7*24*time.Hour-time.Second || defaultTTL > 7*24*time.Hour+time.Second {
		t.Fatalf("expected default TTL around 7 days, got %s", defaultTTL)
	}

	customToken, err := GenerateToken("user_1", "alice", "user", "sig", 120)
	if err != nil {
		t.Fatalf("GenerateToken(custom) error = %v", err)
	}
	customClaims, err := ParseToken(customToken)
	if err != nil {
		t.Fatalf("ParseToken(custom) error = %v", err)
	}
	customTTL := customClaims.ExpiresAt.Time.Sub(customClaims.IssuedAt.Time)
	if customTTL < 119*time.Second || customTTL > 121*time.Second {
		t.Fatalf("expected custom TTL around 120 seconds, got %s", customTTL)
	}
}

func TestValidateTokenRejectsExpiredToken(t *testing.T) {
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
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Hour)),
			NotBefore: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
			Issuer:    "ember-api",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}

	if err := ValidateToken(tokenString); err == nil {
		t.Fatalf("expected expired token to be rejected")
	}
}

func TestComputePasswordSignatureUsesSecretAndHash(t *testing.T) {
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	if err := InitJWT(); err != nil {
		t.Fatalf("InitJWT() error = %v", err)
	}

	sig1 := ComputePasswordSignature("hash_1")
	sig2 := ComputePasswordSignature("hash_1")
	sig3 := ComputePasswordSignature("hash_2")
	if sig1 == "" || len(sig1) != 24 {
		t.Fatalf("expected 12-byte hex signature, got %q", sig1)
	}
	if sig1 != sig2 {
		t.Fatalf("expected same hash and secret to produce stable signature")
	}
	if sig1 == sig3 {
		t.Fatalf("expected different password hashes to produce different signatures")
	}

	t.Setenv("JWT_SECRET", "fedcba9876543210fedcba9876543210")
	if err := InitJWT(); err != nil {
		t.Fatalf("InitJWT() error = %v", err)
	}
	if sig4 := ComputePasswordSignature("hash_1"); sig4 == sig1 {
		t.Fatalf("expected different JWT secret to change password signature")
	}
}

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

func TestParseTokenRejectsMalformedToken(t *testing.T) {
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	if err := InitJWT(); err != nil {
		t.Fatalf("InitJWT() error = %v", err)
	}

	if _, err := ParseToken("not-a-token"); err == nil {
		t.Fatalf("expected malformed token to be rejected")
	}
	if err := ValidateToken("not-a-token"); err == nil {
		t.Fatalf("expected malformed token validation to fail")
	}
}

func TestParseTokenRejectsInvalidClaimsPayload(t *testing.T) {
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	if err := InitJWT(); err != nil {
		t.Fatalf("InitJWT() error = %v", err)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp": "not-a-number",
	})
	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}

	if _, err := ParseToken(tokenString); err == nil {
		t.Fatalf("expected invalid registered claims payload to be rejected")
	} else if errors.Is(err, jwt.ErrTokenSignatureInvalid) {
		t.Fatalf("expected claims validation error, got signature error: %v", err)
	}
}
