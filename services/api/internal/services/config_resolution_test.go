package services

import (
	"testing"

	"github.com/konghang/ember/backend/internal/models"
)

func TestResolveDefinitionPrefersDatabaseOverEnvAndDefault(t *testing.T) {
	t.Setenv("TEST_PRIORITY_KEY", "env-value")

	service := &ConfigService{}
	def := ConfigDefinition{
		Key:          "TEST_PRIORITY_KEY",
		EnvKey:       "TEST_PRIORITY_KEY",
		DefaultValue: "default-value",
		Type:         ConfigValueString,
		Editable:     true,
	}
	settingsMap := map[string]models.Setting{
		def.Key: {
			Key:   def.Key,
			Value: "database-value",
		},
	}

	item, err := service.resolveDefinition(def, settingsMap)
	if err != nil {
		t.Fatalf("resolveDefinition returned error: %v", err)
	}

	if item.Source != ConfigSourceDatabase {
		t.Fatalf("expected database source, got %s", item.Source)
	}
	if !item.HasValue {
		t.Fatal("expected resolved item to have value")
	}
	if item.Value == nil || *item.Value != "database-value" {
		t.Fatalf("expected database value, got %+v", item.Value)
	}
}

func TestResolveDefinitionFallsBackToEnvThenDefaultThenUnset(t *testing.T) {
	t.Setenv("TEST_FALLBACK_KEY", "env-value")

	service := &ConfigService{}
	envDef := ConfigDefinition{
		Key:          "TEST_FALLBACK_KEY",
		EnvKey:       "TEST_FALLBACK_KEY",
		DefaultValue: "default-value",
		Type:         ConfigValueString,
	}

	envItem, err := service.resolveDefinition(envDef, map[string]models.Setting{})
	if err != nil {
		t.Fatalf("expected env resolution to succeed: %v", err)
	}
	if envItem.Source != ConfigSourceEnv {
		t.Fatalf("expected env source, got %s", envItem.Source)
	}
	if envItem.Value == nil || *envItem.Value != "env-value" {
		t.Fatalf("expected env value, got %+v", envItem.Value)
	}

	defaultDef := ConfigDefinition{
		Key:          "TEST_DEFAULT_KEY",
		DefaultValue: "default-value",
		Type:         ConfigValueString,
	}
	defaultItem, err := service.resolveDefinition(defaultDef, map[string]models.Setting{})
	if err != nil {
		t.Fatalf("expected default resolution to succeed: %v", err)
	}
	if defaultItem.Source != ConfigSourceDefault {
		t.Fatalf("expected default source, got %s", defaultItem.Source)
	}
	if defaultItem.Value == nil || *defaultItem.Value != "default-value" {
		t.Fatalf("expected default value, got %+v", defaultItem.Value)
	}

	unsetDef := ConfigDefinition{
		Key:  "TEST_UNSET_KEY",
		Type: ConfigValueString,
	}
	unsetItem, err := service.resolveDefinition(unsetDef, map[string]models.Setting{})
	if err != nil {
		t.Fatalf("expected unset resolution to succeed: %v", err)
	}
	if unsetItem.Source != ConfigSourceUnset {
		t.Fatalf("expected unset source, got %s", unsetItem.Source)
	}
	if unsetItem.HasValue {
		t.Fatal("unset item should not have value")
	}
	if unsetItem.Value != nil {
		t.Fatalf("unset item should not expose value, got %+v", unsetItem.Value)
	}
}

func TestResolveDefinitionHidesSensitiveValues(t *testing.T) {
	t.Setenv("TEST_SECRET_KEY", "env-secret")

	service := &ConfigService{}
	def := ConfigDefinition{
		Key:       "TEST_SECRET_KEY",
		EnvKey:    "TEST_SECRET_KEY",
		Type:      ConfigValueSecret,
		Sensitive: true,
	}

	item, err := service.resolveDefinition(def, map[string]models.Setting{})
	if err != nil {
		t.Fatalf("expected sensitive resolution to succeed: %v", err)
	}
	if item.Source != ConfigSourceEnv {
		t.Fatalf("expected env source, got %s", item.Source)
	}
	if !item.HasValue {
		t.Fatal("expected sensitive item to report hasValue=true")
	}
	if item.Value != nil {
		t.Fatalf("sensitive item should not expose value, got %+v", item.Value)
	}
}

func TestResolveDefinitionReportsEncryptedValueErrorWhenKeyMissing(t *testing.T) {
	encryptedService := &ConfigService{encryptionKey: "correct-key"}
	encryptedValue, err := encryptedService.encrypt("super-secret")
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	serviceWithoutKey := &ConfigService{}
	def := ConfigDefinition{
		Key:       "TEST_ENCRYPTED_SECRET",
		Type:      ConfigValueSecret,
		Sensitive: true,
	}
	settingsMap := map[string]models.Setting{
		def.Key: {
			Key:         def.Key,
			Value:       encryptedValue,
			IsEncrypted: true,
		},
	}

	item, resolveErr := serviceWithoutKey.resolveDefinition(def, settingsMap)
	if resolveErr != nil {
		t.Fatalf("resolveDefinition should surface decrypt failure in item error, got: %v", resolveErr)
	}
	if item.Source != ConfigSourceDatabase {
		t.Fatalf("expected database source, got %s", item.Source)
	}
	if item.Error != ErrConfigEncryptionKeyMissing.Error() {
		t.Fatalf("expected encryption error, got %q", item.Error)
	}
}

func TestResolveDefinitionAllowsExplicitEmptyDatabaseOverride(t *testing.T) {
	t.Setenv("TEST_ALLOW_EMPTY_KEY", "env-value")

	service := &ConfigService{}
	def := ConfigDefinition{
		Key:        "TEST_ALLOW_EMPTY_KEY",
		EnvKey:     "TEST_ALLOW_EMPTY_KEY",
		Type:       ConfigValueString,
		AllowEmpty: true,
	}
	settingsMap := map[string]models.Setting{
		def.Key: {
			Key:   def.Key,
			Value: "",
		},
	}

	item, err := service.resolveDefinition(def, settingsMap)
	if err != nil {
		t.Fatalf("resolveDefinition returned error: %v", err)
	}
	if item.Source != ConfigSourceDatabase {
		t.Fatalf("expected database source, got %s", item.Source)
	}
	if item.HasValue {
		t.Fatal("empty override should report hasValue=false")
	}
	if item.Value == nil || *item.Value != "" {
		t.Fatalf("expected explicit empty string, got %+v", item.Value)
	}
}
