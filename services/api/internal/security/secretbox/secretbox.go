package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"

	"golang.org/x/crypto/hkdf"
)

var (
	ErrKeyMissing     = errors.New("encryption key is missing")
	ErrPurposeMissing = errors.New("encryption purpose is missing")
	ErrDecrypt        = errors.New("secret decryption failed")
)

// Box encrypts database secrets with the format historically used by ConfigService.
type Box struct {
	key [sha256.Size]byte
}

// New derives an AES-256 key from the deployment encryption key.
func New(rawKey string) (*Box, error) {
	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return nil, ErrKeyMissing
	}
	return &Box{key: sha256.Sum256([]byte(rawKey))}, nil
}

// NewDerived creates a purpose-separated box while keeping the AES-GCM payload format.
func NewDerived(rawKey, purpose string) (*Box, error) {
	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return nil, ErrKeyMissing
	}
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		return nil, ErrPurposeMissing
	}

	var key [sha256.Size]byte
	reader := hkdf.New(sha256.New, []byte(rawKey), nil, []byte("ember/secretbox/"+purpose))
	if _, err := io.ReadFull(reader, key[:]); err != nil {
		return nil, err
	}
	return &Box{key: key}, nil
}

// Encrypt seals plaintext with AES-GCM and a fresh nonce.
func (b *Box) Encrypt(plain string) (string, error) {
	gcm, err := b.gcm()
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(append(nonce, ciphertext...)), nil
}

// Decrypt opens a base64-encoded AES-GCM payload without exposing low-level errors.
func (b *Box) Decrypt(encoded string) (string, error) {
	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", ErrDecrypt
	}

	gcm, err := b.gcm()
	if err != nil {
		return "", ErrDecrypt
	}
	if len(payload) < gcm.NonceSize() {
		return "", ErrDecrypt
	}

	nonce := payload[:gcm.NonceSize()]
	ciphertext := payload[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", ErrDecrypt
	}
	return string(plain), nil
}

func (b *Box) gcm() (cipher.AEAD, error) {
	block, err := aes.NewCipher(b.key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
