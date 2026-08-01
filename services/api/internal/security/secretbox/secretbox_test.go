package secretbox

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestNewRejectsMissingKey(t *testing.T) {
	if _, err := New("  "); !errors.Is(err, ErrKeyMissing) {
		t.Fatalf("New() error = %v, want ErrKeyMissing", err)
	}
}

func TestBoxEncryptDecryptRoundTrip(t *testing.T) {
	box, err := New("test-encryption-key")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	plain := "UID=fake; CID=fake; SEID=fake"
	first, err := box.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt() failed: %v", err)
	}
	second, err := box.Encrypt(plain)
	if err != nil {
		t.Fatalf("second Encrypt() failed: %v", err)
	}
	if first == plain || strings.Contains(first, plain) {
		t.Fatal("ciphertext must not expose plaintext")
	}
	if first == second {
		t.Fatal("independent encryptions must use different nonces")
	}

	got, err := box.Decrypt(first)
	if err != nil {
		t.Fatalf("Decrypt() failed: %v", err)
	}
	if got != plain {
		t.Fatalf("Decrypt() = %q, want %q", got, plain)
	}
}

func TestBoxDecryptRejectsWrongKeyAndMalformedPayload(t *testing.T) {
	box, err := New("correct-key")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	ciphertext, err := box.Encrypt("sensitive-value")
	if err != nil {
		t.Fatalf("Encrypt() failed: %v", err)
	}

	wrongBox, err := New("wrong-key")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if _, err := wrongBox.Decrypt(ciphertext); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("wrong-key Decrypt() error = %v, want ErrDecrypt", err)
	}
	if _, err := box.Decrypt("not-base64"); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("malformed Decrypt() error = %v, want ErrDecrypt", err)
	}
	if _, err := box.Decrypt(base64.StdEncoding.EncodeToString([]byte("short"))); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("short Decrypt() error = %v, want ErrDecrypt", err)
	}
}

func TestBoxDecryptsLegacyConfigCiphertext(t *testing.T) {
	const key = "legacy-config-key"
	const plain = "legacy-sensitive-setting"

	box, err := New(key)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	legacyCiphertext := encryptWithLegacyConfigFormat(t, key, plain)

	got, err := box.Decrypt(legacyCiphertext)
	if err != nil {
		t.Fatalf("Decrypt() legacy ciphertext failed: %v", err)
	}
	if got != plain {
		t.Fatalf("Decrypt() = %q, want %q", got, plain)
	}
}

func TestNewDerivedSeparatesCredentialPurposes(t *testing.T) {
	configBox, err := New("shared-root-key")
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	p115Box, err := NewDerived("shared-root-key", "p115-cookie")
	if err != nil {
		t.Fatalf("NewDerived() failed: %v", err)
	}
	otherBox, err := NewDerived("shared-root-key", "other-credential")
	if err != nil {
		t.Fatalf("NewDerived() failed: %v", err)
	}

	ciphertext, err := p115Box.Encrypt("cookie-secret")
	if err != nil {
		t.Fatalf("Encrypt() failed: %v", err)
	}
	if got, err := p115Box.Decrypt(ciphertext); err != nil || got != "cookie-secret" {
		t.Fatalf("derived round trip = %q, %v", got, err)
	}
	if _, err := configBox.Decrypt(ciphertext); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("legacy config box decrypted p115 ciphertext: %v", err)
	}
	if _, err := otherBox.Decrypt(ciphertext); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("other purpose box decrypted p115 ciphertext: %v", err)
	}
}

func TestNewDerivedRequiresPurpose(t *testing.T) {
	if _, err := NewDerived("shared-root-key", " "); !errors.Is(err, ErrPurposeMissing) {
		t.Fatalf("NewDerived() error = %v, want ErrPurposeMissing", err)
	}
}

func encryptWithLegacyConfigFormat(t *testing.T, rawKey, plain string) string {
	t.Helper()

	key := sha256.Sum256([]byte(rawKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		t.Fatalf("aes.NewCipher() failed: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("cipher.NewGCM() failed: %v", err)
	}
	nonce := bytes.Repeat([]byte{0x2a}, gcm.NonceSize())
	ciphertext := gcm.Seal(nil, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(append(nonce, ciphertext...))
}
