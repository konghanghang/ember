package envbootstrap

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestLoadUsesExplicitDotenvBeforeLoggingConsumers(t *testing.T) {
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	originalLevel, levelPresent := os.LookupEnv("LOG_LEVEL")
	originalDotenv, dotenvPresent := os.LookupEnv("EMBER_DOTENV")
	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
		if levelPresent {
			_ = os.Setenv("LOG_LEVEL", originalLevel)
		} else {
			_ = os.Unsetenv("LOG_LEVEL")
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
	if err := os.WriteFile(path, []byte("LOG_LEVEL=debug\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Unsetenv("LOG_LEVEL"); err != nil {
		t.Fatalf("Unsetenv(LOG_LEVEL) error = %v", err)
	}
	if err := os.Setenv("EMBER_DOTENV", path); err != nil {
		t.Fatalf("Setenv(EMBER_DOTENV) error = %v", err)
	}
	loadOnce = sync.Once{}
	loadResult = Result{}

	result := Load()
	if result.Path != path || result.Err != nil || os.Getenv("LOG_LEVEL") != "debug" {
		t.Fatalf("Load()=%+v LOG_LEVEL=%q", result, os.Getenv("LOG_LEVEL"))
	}
}

func TestLoadDefaultsToNearestDotenvWithoutOverwritingEnvironment(t *testing.T) {
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	originalLevel, levelPresent := os.LookupEnv("LOG_LEVEL")
	originalDotenv, dotenvPresent := os.LookupEnv("EMBER_DOTENV")
	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
		if levelPresent {
			_ = os.Setenv("LOG_LEVEL", originalLevel)
		} else {
			_ = os.Unsetenv("LOG_LEVEL")
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
	if err := os.WriteFile(filepath.Join(tempDir, ".env"), []byte("LOG_LEVEL=debug\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	if err := os.Unsetenv("EMBER_DOTENV"); err != nil {
		t.Fatalf("Unsetenv(EMBER_DOTENV) error = %v", err)
	}
	if err := os.Setenv("LOG_LEVEL", "info"); err != nil {
		t.Fatalf("Setenv(LOG_LEVEL) error = %v", err)
	}
	loadOnce = sync.Once{}
	loadResult = Result{}

	result := Load()
	if result.Path != ".env" || result.Err != nil || os.Getenv("LOG_LEVEL") != "info" {
		t.Fatalf("Load()=%+v LOG_LEVEL=%q", result, os.Getenv("LOG_LEVEL"))
	}
}
