package tokenhash

import (
	"encoding/hex"
	"errors"
	"testing"
)

func TestHasherMatchesFixedPurposeSeparatedVector(t *testing.T) {
	hasher, err := New("fixture-root-key", "emby-access-token")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	digest, err := hasher.Sum("fixture-access-token")
	if err != nil {
		t.Fatalf("Sum() error = %v", err)
	}
	if got := hex.EncodeToString(digest); got != "acbb92b655936a139c7dd5729ed818944aa252b1cc9766d762d4119c637b8cd4" {
		t.Fatalf("Sum() = %s", got)
	}
	other, err := New("fixture-root-key", "other-purpose")
	if err != nil {
		t.Fatalf("New(other) error = %v", err)
	}
	otherDigest, err := other.Sum("fixture-access-token")
	if err != nil {
		t.Fatalf("other.Sum() error = %v", err)
	}
	if string(digest) == string(otherDigest) {
		t.Fatal("purpose separation did not change token digest")
	}
}

func TestHasherRejectsMissingInputs(t *testing.T) {
	if _, err := New("", "emby-access-token"); !errors.Is(err, ErrKeyMissing) {
		t.Fatalf("New(empty key) error = %v", err)
	}
	if _, err := New("key", ""); !errors.Is(err, ErrPurposeMissing) {
		t.Fatalf("New(empty purpose) error = %v", err)
	}
	hasher, err := New("key", "emby-access-token")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := hasher.Sum(""); !errors.Is(err, ErrTokenMissing) {
		t.Fatalf("Sum(empty token) error = %v", err)
	}
	if _, err := hasher.Sum(" token "); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("Sum(padded token) error = %v", err)
	}
}
