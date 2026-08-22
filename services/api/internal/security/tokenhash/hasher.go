// Package tokenhash provides purpose-separated HMAC digests for high-entropy
// external access tokens that must never be persisted in plaintext.
package tokenhash

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"io"
	"strings"

	"golang.org/x/crypto/hkdf"
)

const maxTokenLength = 16 * 1024

var (
	ErrKeyMissing     = errors.New("token hash key is missing")
	ErrPurposeMissing = errors.New("token hash purpose is missing")
	ErrTokenMissing   = errors.New("access token is missing")
	ErrTokenInvalid   = errors.New("access token is invalid")
)

// Hasher computes one-way SHA-256 HMAC digests using a purpose-derived key.
type Hasher struct {
	key [sha256.Size]byte
}

// New derives a token-HMAC key from the deployment root key and one purpose.
func New(rawKey, purpose string) (*Hasher, error) {
	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return nil, ErrKeyMissing
	}
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		return nil, ErrPurposeMissing
	}
	var key [sha256.Size]byte
	reader := hkdf.New(sha256.New, []byte(rawKey), nil, []byte("ember/token-hmac/"+purpose))
	if _, err := io.ReadFull(reader, key[:]); err != nil {
		return nil, err
	}
	return &Hasher{key: key}, nil
}

// Sum hashes an exact token value; surrounding whitespace and header
// injection are rejected rather than silently normalized.
func (hasher *Hasher) Sum(token string) ([]byte, error) {
	if token == "" {
		return nil, ErrTokenMissing
	}
	if len(token) > maxTokenLength || strings.TrimSpace(token) != token || strings.ContainsAny(token, "\r\n") {
		return nil, ErrTokenInvalid
	}
	digest := hmac.New(sha256.New, hasher.key[:])
	_, _ = digest.Write([]byte(token))
	return digest.Sum(nil), nil
}
