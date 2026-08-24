// Package envbootstrap loads the optional Go process dotenv before logging and
// other startup consumers resolve environment-backed configuration.
package envbootstrap

import (
	"os"
	"sync"

	"github.com/joho/godotenv"
)

// Result describes the single process-wide dotenv attempt without exposing
// file contents or changing the selected path after startup.
type Result struct {
	Path string
	Err  error
}

var (
	loadOnce   sync.Once
	loadResult Result
)

// Load applies the explicit EMBER_DOTENV file or the nearest conventional
// project file exactly once. Existing process variables retain precedence.
func Load() Result {
	loadOnce.Do(func() {
		path := os.Getenv("EMBER_DOTENV")
		if path == "" {
			path = defaultDotenvPath()
		}
		if path == "" {
			return
		}
		loadResult.Path = path
		loadResult.Err = godotenv.Load(path)
	})
	return loadResult
}

// defaultDotenvPath preserves the historical root-or-service lookup order.
func defaultDotenvPath() string {
	for _, path := range []string{".env", "services/api/.env"} {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}
