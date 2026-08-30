package envbootstrap

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestLoadUsesExplicitDotenvBeforeProcessConsumers(t *testing.T) {
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	originalValue, valuePresent := os.LookupEnv("EMBER_ENVBOOTSTRAP_TEST_VALUE")
	originalDotenv, dotenvPresent := os.LookupEnv("EMBER_DOTENV")
	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
		if valuePresent {
			_ = os.Setenv("EMBER_ENVBOOTSTRAP_TEST_VALUE", originalValue)
		} else {
			_ = os.Unsetenv("EMBER_ENVBOOTSTRAP_TEST_VALUE")
		}
		if dotenvPresent {
			_ = os.Setenv("EMBER_DOTENV", originalDotenv)
		} else {
			_ = os.Unsetenv("EMBER_DOTENV")
		}
		loadOnce = sync.Once{}
		loadResult = Result{}
	})

	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "gateway.env")
	if err := os.WriteFile(path, []byte("EMBER_ENVBOOTSTRAP_TEST_VALUE=dotenv\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Unsetenv("EMBER_ENVBOOTSTRAP_TEST_VALUE"); err != nil {
		t.Fatalf("Unsetenv(EMBER_ENVBOOTSTRAP_TEST_VALUE) error = %v", err)
	}
	if err := os.Setenv("EMBER_DOTENV", path); err != nil {
		t.Fatalf("Setenv(EMBER_DOTENV) error = %v", err)
	}
	loadOnce = sync.Once{}
	loadResult = Result{}

	result := Load()
	if result.Path != path || result.Err != nil || os.Getenv("EMBER_ENVBOOTSTRAP_TEST_VALUE") != "dotenv" {
		t.Fatalf("Load()=%+v value=%q", result, os.Getenv("EMBER_ENVBOOTSTRAP_TEST_VALUE"))
	}
}

func TestLoadDefaultsToNearestDotenvWithoutOverwritingEnvironment(t *testing.T) {
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	originalValue, valuePresent := os.LookupEnv("EMBER_ENVBOOTSTRAP_TEST_VALUE")
	originalDotenv, dotenvPresent := os.LookupEnv("EMBER_DOTENV")
	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
		if valuePresent {
			_ = os.Setenv("EMBER_ENVBOOTSTRAP_TEST_VALUE", originalValue)
		} else {
			_ = os.Unsetenv("EMBER_ENVBOOTSTRAP_TEST_VALUE")
		}
		if dotenvPresent {
			_ = os.Setenv("EMBER_DOTENV", originalDotenv)
		} else {
			_ = os.Unsetenv("EMBER_DOTENV")
		}
		loadOnce = sync.Once{}
		loadResult = Result{}
	})

	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, ".env"), []byte("EMBER_ENVBOOTSTRAP_TEST_VALUE=dotenv\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	if err := os.Unsetenv("EMBER_DOTENV"); err != nil {
		t.Fatalf("Unsetenv(EMBER_DOTENV) error = %v", err)
	}
	if err := os.Setenv("EMBER_ENVBOOTSTRAP_TEST_VALUE", "process"); err != nil {
		t.Fatalf("Setenv(EMBER_ENVBOOTSTRAP_TEST_VALUE) error = %v", err)
	}
	loadOnce = sync.Once{}
	loadResult = Result{}

	result := Load()
	if result.Path != ".env" || result.Err != nil || os.Getenv("EMBER_ENVBOOTSTRAP_TEST_VALUE") != "process" {
		t.Fatalf("Load()=%+v value=%q", result, os.Getenv("EMBER_ENVBOOTSTRAP_TEST_VALUE"))
	}
}
