package logging

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	logDir             = "logs"
	ProcessRoleAPI     = "api"
	ProcessRoleGateway = "gateway"
)

// Level is the first project-wide application logging boundary. Historical
// log.Printf calls remain Info-compatible while newly identified high-volume
// diagnostics can be gated behind Debug.
type Level uint32

const (
	LevelInfo Level = iota
	LevelDebug
)

var ErrInvalidProcessRole = errors.New("logging process role invalid")

type dailyFileWriter struct {
	baseDir     string
	filePrefix  string
	stdout      io.Writer
	mu          sync.Mutex
	currentDate string
	currentFile *os.File
}

func newDailyFileWriter(baseDir, filePrefix string) *dailyFileWriter {
	return &dailyFileWriter{
		baseDir:    baseDir,
		filePrefix: filePrefix,
		stdout:     os.Stdout,
	}
}

func (w *dailyFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	written, stdoutErr := w.stdout.Write(p)

	file, err := w.ensureFile()
	if err != nil {
		if stdoutErr != nil {
			return written, stdoutErr
		}
		return written, err
	}

	if _, fileErr := file.Write(p); fileErr != nil {
		if stdoutErr != nil {
			return written, stdoutErr
		}
		return written, fileErr
	}

	if stdoutErr != nil {
		return written, stdoutErr
	}
	return written, nil
}

func (w *dailyFileWriter) ensureFile() (*os.File, error) {
	if err := os.MkdirAll(w.baseDir, 0o755); err != nil {
		return nil, err
	}

	currentDate := time.Now().Format("2006-01-02")
	if w.currentFile != nil && w.currentDate == currentDate {
		return w.currentFile, nil
	}

	if w.currentFile != nil {
		_ = w.currentFile.Close()
		w.currentFile = nil
	}

	path := filepath.Join(w.baseDir, fmt.Sprintf("%s-%s.log", w.filePrefix, currentDate))
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}

	w.currentDate = currentDate
	w.currentFile = file
	return file, nil
}

var (
	initOnce               = &sync.Once{}
	logWriter    io.Writer = os.Stdout
	initErr      error
	currentLevel atomic.Uint32
)

// parseLevel accepts the two currently safe project-wide levels. Invalid
// values fall back to Info so a logging typo never takes down an Ember service.
func parseLevel(raw string) (Level, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "info":
		return LevelInfo, false
	case "debug":
		return LevelDebug, false
	default:
		return LevelInfo, true
	}
}

// levelName returns the stable lower-case deployment value used in logs.
func levelName(level Level) string {
	if level == LevelDebug {
		return "debug"
	}
	return "info"
}

// DebugEnabled reports the process-wide resolved level without reading the
// environment again, keeping API, GORM, Gin and Gateway assembly consistent.
func DebugEnabled() bool {
	return Level(currentLevel.Load()) == LevelDebug
}

// Debugf writes a diagnostic only while the process-wide Debug level is active.
func Debugf(format string, args ...interface{}) {
	if DebugEnabled() {
		log.Printf(format, args...)
	}
}

// Infof preserves the existing always-visible application event boundary.
func Infof(format string, args ...interface{}) {
	log.Printf(format, args...)
}

// processLogPrefix maps the two production process roles to distinct file
// prefixes so direct non-Docker execution cannot mix API and Gateway logs.
func processLogPrefix(processRole string) (string, error) {
	switch processRole {
	case ProcessRoleAPI, ProcessRoleGateway:
		return processRole, nil
	default:
		return "", ErrInvalidProcessRole
	}
}

// Init configures stdout and daily file logging for one validated process role.
func Init(processRole string) error {
	filePrefix, err := processLogPrefix(processRole)
	if err != nil {
		return err
	}
	initOnce.Do(func() {
		level, invalid := parseLevel(os.Getenv("LOG_LEVEL"))
		currentLevel.Store(uint32(level))
		writer := newDailyFileWriter(logDir, filePrefix)
		logWriter = writer
		log.SetOutput(writer)
		log.SetFlags(log.LstdFlags | log.Lmicroseconds)
		gin.DefaultWriter = writer
		gin.DefaultErrorWriter = writer
		initErr = os.MkdirAll(logDir, 0o755)
		if invalid {
			log.Printf("[Logging] level=info code=log_level_invalid fallbackLevel=info")
		}
		log.Printf("[Logging] level=info code=logging_initialized processRole=%s logLevel=%s", processRole, levelName(level))
	})
	return initErr
}

func Writer() io.Writer {
	return logWriter
}

const apiSlowRequestThreshold = 2 * time.Second

// GinAccessLogger records safe API access metadata. Successful requests are
// Debug-only; failures and slow requests remain visible at the default level.
// Query values, headers, cookies and bodies are never copied into the message.
func GinAccessLogger() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		startedAt := time.Now()
		ctx.Next()
		duration := time.Since(startedAt)
		statusCode := ctx.Writer.Status()
		path := boundedLogText(ctx.FullPath(), 1024)
		if path == "" {
			path = "unmatched"
		}
		method := ""
		if ctx.Request != nil {
			method = boundedLogText(ctx.Request.Method, 32)
		}
		if (path == "/" || path == "/health") && statusCode < http.StatusBadRequest {
			return
		}
		if statusCode >= http.StatusBadRequest {
			Infof("[API] level=info code=request_failed method=%s path=%q statusCode=%d durationMs=%d",
				method, path, statusCode, duration.Milliseconds())
			return
		}
		if duration >= apiSlowRequestThreshold {
			Infof("[API] level=info code=request_slow method=%s path=%q statusCode=%d durationMs=%d",
				method, path, statusCode, duration.Milliseconds())
			return
		}
		Debugf("[API] level=debug code=request_completed method=%s path=%q statusCode=%d durationMs=%d",
			method, path, statusCode, duration.Milliseconds())
	}
}

// boundedLogText prevents client-controlled access metadata from producing an
// unbounded log line.
func boundedLogText(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	return value[:maxBytes]
}
