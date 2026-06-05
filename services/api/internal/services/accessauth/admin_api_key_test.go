package accessauth

import (
	"errors"
	"testing"
)

type fakeAdminAPIKeyStore struct {
	hash       string
	loadErr    error
	saveErr    error
	clearErr   error
	savedBy    string
	clearedBy  string
	savedHash  string
	clearCalls int
}

func (s *fakeAdminAPIKeyStore) LoadHash() (string, error) {
	if s.loadErr != nil {
		return "", s.loadErr
	}
	return s.hash, nil
}

func (s *fakeAdminAPIKeyStore) SaveHash(hash string, updatedByUserID string) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.hash = hash
	s.savedHash = hash
	s.savedBy = updatedByUserID
	return nil
}

func (s *fakeAdminAPIKeyStore) ClearHash(updatedByUserID string) error {
	if s.clearErr != nil {
		return s.clearErr
	}
	s.hash = ""
	s.clearedBy = updatedByUserID
	s.clearCalls++
	return nil
}

func TestAdminAPIKeyServiceGenerateStoresHashAndReturnsPlainOnce(t *testing.T) {
	store := &fakeAdminAPIKeyStore{}
	service := newAdminAPIKeyServiceWithStore(store)

	result, err := service.Generate("admin_1")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if !result.Configured {
		t.Fatalf("expected generated result to be configured")
	}
	if !LooksLikeAdminAPIKey(result.APIKey) {
		t.Fatalf("expected generated key to use admin api key format, got %q", result.APIKey)
	}
	if store.savedBy != "admin_1" {
		t.Fatalf("expected updatedByUserID to be preserved, got %q", store.savedBy)
	}
	if store.savedHash == "" || store.savedHash == result.APIKey {
		t.Fatalf("expected only hash to be stored, savedHash=%q apiKey=%q", store.savedHash, result.APIKey)
	}
	if store.savedHash != HashAdminAPIKey(result.APIKey) {
		t.Fatalf("stored hash does not match generated api key")
	}
}

func TestAdminAPIKeyServiceStatusAndDisable(t *testing.T) {
	store := &fakeAdminAPIKeyStore{hash: HashAdminAPIKey("ember_sk_existing_key_with_enough_entropy")}
	service := newAdminAPIKeyServiceWithStore(store)

	status, err := service.Status()
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if !status.Configured {
		t.Fatalf("expected configured status")
	}

	status, err = service.Disable("admin_2")
	if err != nil {
		t.Fatalf("Disable failed: %v", err)
	}
	if status.Configured {
		t.Fatalf("expected disabled status")
	}
	if store.clearCalls != 1 || store.clearedBy != "admin_2" || store.hash != "" {
		t.Fatalf("disable did not clear hash correctly: %+v", store)
	}
}

func TestAdminAPIKeyServiceValidate(t *testing.T) {
	key := "ember_sk_valid_key_with_enough_entropy_for_test"
	service := newAdminAPIKeyServiceWithStore(&fakeAdminAPIKeyStore{hash: HashAdminAPIKey(key)})

	ok, err := service.Validate(key)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected matching api key to validate")
	}

	ok, err = service.Validate("ember_sk_wrong_key_with_enough_entropy_for_test")
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if ok {
		t.Fatalf("expected mismatched api key to be rejected")
	}

	ok, err = service.Validate("not-an-admin-key")
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if ok {
		t.Fatalf("expected invalid format to be rejected")
	}
}

func TestAdminAPIKeyServicePropagatesStoreErrors(t *testing.T) {
	loadErr := errors.New("db unavailable")
	service := newAdminAPIKeyServiceWithStore(&fakeAdminAPIKeyStore{loadErr: loadErr})

	if _, err := service.Status(); !errors.Is(err, loadErr) {
		t.Fatalf("expected status load error, got %v", err)
	}

	if _, err := service.Validate("ember_sk_key_with_enough_entropy_for_test"); !errors.Is(err, loadErr) {
		t.Fatalf("expected validate load error, got %v", err)
	}
}
