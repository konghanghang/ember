package logging

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestParseLevelDefaultsAndNormalizes(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		want        Level
		wantInvalid bool
	}{
		{name: "empty defaults to info", want: LevelInfo},
		{name: "info", raw: " info ", want: LevelInfo},
		{name: "debug", raw: "DeBuG", want: LevelDebug},
		{name: "invalid falls back", raw: "verbose", want: LevelInfo, wantInvalid: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, invalid := parseLevel(test.raw)
			if got != test.want || invalid != test.wantInvalid {
				t.Fatalf("parseLevel(%q)=(%q,%t), want (%q,%t)", test.raw, got, invalid, test.want, test.wantInvalid)
			}
		})
	}
}

func TestDebugfHonorsGlobalLevel(t *testing.T) {
	originalOutput := log.Writer()
	originalLevel := currentLevel.Load()
	t.Cleanup(func() {
		log.SetOutput(originalOutput)
		currentLevel.Store(originalLevel)
	})

	var output bytes.Buffer
	log.SetOutput(&output)
	currentLevel.Store(uint32(LevelInfo))
	Debugf("code=debug_hidden")
	Infof("code=info_visible")
	if strings.Contains(output.String(), "debug_hidden") || !strings.Contains(output.String(), "info_visible") {
		t.Fatalf("info output=%q", output.String())
	}

	output.Reset()
	currentLevel.Store(uint32(LevelDebug))
	Debugf("code=debug_visible")
	if !strings.Contains(output.String(), "debug_visible") {
		t.Fatalf("debug output=%q", output.String())
	}
}

func TestApplyLevelSwitchesDynamicallyAndRejectsInvalidValue(t *testing.T) {
	originalLevel := currentLevel.Load()
	t.Cleanup(func() { currentLevel.Store(originalLevel) })
	currentLevel.Store(uint32(LevelInfo))

	if err := ApplyLevel(" DeBuG "); err != nil || !DebugEnabled() {
		t.Fatalf("ApplyLevel(debug) error=%v debug=%t", err, DebugEnabled())
	}
	if err := ApplyLevel("trace"); !errors.Is(err, ErrInvalidLevel) {
		t.Fatalf("ApplyLevel(trace) error=%v, want ErrInvalidLevel", err)
	}
	if !DebugEnabled() {
		t.Fatal("invalid level changed the last valid runtime level")
	}
	if err := ApplyLevel("info"); err != nil || DebugEnabled() {
		t.Fatalf("ApplyLevel(info) error=%v debug=%t", err, DebugEnabled())
	}
}

func TestSyncLevelAppliesDatabaseValueAndKeepsLastLevelOnFailure(t *testing.T) {
	originalOutput := log.Writer()
	originalLevel := currentLevel.Load()
	t.Cleanup(func() {
		log.SetOutput(originalOutput)
		currentLevel.Store(originalLevel)
	})
	var output bytes.Buffer
	log.SetOutput(&output)
	currentLevel.Store(uint32(LevelInfo))

	provider := &stubLevelProvider{level: "debug"}
	SyncLevel(context.Background(), ProcessRoleGateway, provider)
	if !DebugEnabled() || !strings.Contains(output.String(), "code=log_level_loaded") ||
		!strings.Contains(output.String(), "source=system_config") {
		t.Fatalf("sync success debug=%t logs=%q", DebugEnabled(), output.String())
	}

	output.Reset()
	provider.err = errors.New("database unavailable with secret-value")
	SyncLevel(context.Background(), ProcessRoleGateway, provider)
	if !DebugEnabled() || !strings.Contains(output.String(), "code=log_level_load_failed") ||
		strings.Contains(output.String(), "secret-value") {
		t.Fatalf("sync failure debug=%t logs=%q", DebugEnabled(), output.String())
	}
}

type stubLevelProvider struct {
	level string
	err   error
}

func (provider *stubLevelProvider) LogLevel(context.Context) (string, error) {
	return provider.level, provider.err
}

func TestLogInitializedReportsResolvedLevel(t *testing.T) {
	originalOutput := log.Writer()
	originalLevel := currentLevel.Load()
	t.Cleanup(func() {
		log.SetOutput(originalOutput)
		currentLevel.Store(originalLevel)
	})

	var output bytes.Buffer
	log.SetOutput(&output)
	currentLevel.Store(uint32(LevelDebug))
	LogInitialized(ProcessRoleGateway)
	if !strings.Contains(output.String(), "processRole=gateway logLevel=debug source=bootstrap_default") {
		t.Fatalf("initialized output=%q", output.String())
	}
}

func TestGinAccessLoggerUsesGlobalLevelWithoutQueryValues(t *testing.T) {
	originalOutput := log.Writer()
	originalLevel := currentLevel.Load()
	t.Cleanup(func() {
		log.SetOutput(originalOutput)
		currentLevel.Store(originalLevel)
	})

	var output bytes.Buffer
	log.SetOutput(&output)
	router := gin.New()
	router.Use(GinAccessLogger())
	router.GET("/items", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.GET("/failed", func(c *gin.Context) { c.Status(http.StatusBadGateway) })
	router.GET("/redeem/:code", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	currentLevel.Store(uint32(LevelInfo))
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/items?api_key=secret", nil))
	if output.Len() != 0 {
		t.Fatalf("info success log=%q, want empty", output.String())
	}
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/failed?api_key=secret", nil))
	if !strings.Contains(output.String(), "code=request_failed") || strings.Contains(output.String(), "secret") {
		t.Fatalf("info failure log=%q", output.String())
	}

	output.Reset()
	currentLevel.Store(uint32(LevelDebug))
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/items?api_key=secret", nil))
	if !strings.Contains(output.String(), "code=request_completed") || strings.Contains(output.String(), "secret") {
		t.Fatalf("debug access log=%q", output.String())
	}

	output.Reset()
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/redeem/sensitive-redemption-code", nil))
	if !strings.Contains(output.String(), `path="/redeem/:code"`) || strings.Contains(output.String(), "sensitive-redemption-code") {
		t.Fatalf("debug path parameter log=%q", output.String())
	}
}

func TestDailyFileWriterWritesStdoutAndDailyLogFile(t *testing.T) {
	var stdout bytes.Buffer
	writer := newDailyFileWriter(t.TempDir(), "app")
	writer.stdout = &stdout

	message := []byte("hello log\n")
	written, err := writer.Write(message)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if written != len(message) {
		t.Fatalf("Write() wrote %d bytes, want %d", written, len(message))
	}
	if stdout.String() != string(message) {
		t.Fatalf("stdout mismatch: got %q", stdout.String())
	}

	path := filepath.Join(writer.baseDir, "app-"+time.Now().Format("2006-01-02")+".log")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	if string(content) != string(message) {
		t.Fatalf("file content mismatch: got %q", string(content))
	}
}

func TestDailyFileWriterReusesCurrentDateFile(t *testing.T) {
	writer := newDailyFileWriter(t.TempDir(), "app")
	writer.stdout = &bytes.Buffer{}

	first, err := writer.ensureFile()
	if err != nil {
		t.Fatalf("ensureFile() first error = %v", err)
	}
	second, err := writer.ensureFile()
	if err != nil {
		t.Fatalf("ensureFile() second error = %v", err)
	}
	if first != second {
		t.Fatalf("expected same file for same date")
	}
}

func TestProcessLogPrefix(t *testing.T) {
	tests := []struct {
		role string
		want string
	}{
		{role: ProcessRoleAPI, want: "api"},
		{role: ProcessRoleGateway, want: "gateway"},
	}
	for _, test := range tests {
		t.Run(test.role, func(t *testing.T) {
			got, err := processLogPrefix(test.role)
			if err != nil || got != test.want {
				t.Fatalf("processLogPrefix(%q) = (%q, %v), want (%q, nil)", test.role, got, err, test.want)
			}
		})
	}
	if _, err := processLogPrefix("unknown"); err == nil {
		t.Fatal("processLogPrefix(unknown) error = nil")
	}
}

func TestInitSetsGlobalWriters(t *testing.T) {
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	originalInitOnce := initOnce
	originalLogWriter := logWriter
	originalInitErr := initErr
	originalLogOutput := log.Writer()
	originalLogFlags := log.Flags()
	originalGinWriter := gin.DefaultWriter
	originalGinErrorWriter := gin.DefaultErrorWriter
	originalLevel := currentLevel.Load()
	roleMu.RLock()
	originalRole := currentRole
	roleMu.RUnlock()

	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
		initOnce = originalInitOnce
		logWriter = originalLogWriter
		initErr = originalInitErr
		log.SetOutput(originalLogOutput)
		log.SetFlags(originalLogFlags)
		gin.DefaultWriter = originalGinWriter
		gin.DefaultErrorWriter = originalGinErrorWriter
		currentLevel.Store(originalLevel)
		roleMu.Lock()
		currentRole = originalRole
		roleMu.Unlock()
	})
	t.Setenv("LOG_LEVEL", "debug")

	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	initOnce = &sync.Once{}
	logWriter = os.Stdout
	initErr = nil

	if err := Init(ProcessRoleGateway); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if DebugEnabled() {
		t.Fatal("Init must ignore deployment LOG_LEVEL and keep database default info")
	}
	if Writer() == os.Stdout {
		t.Fatalf("expected Init to replace default writer")
	}
	if _, ok := Writer().(*dailyFileWriter); !ok {
		t.Fatalf("expected dailyFileWriter, got %T", Writer())
	}
	if _, err := os.Stat(logDir); err != nil {
		t.Fatalf("expected log dir %q to exist: %v", logDir, err)
	}
	writer, ok := log.Writer().(*dailyFileWriter)
	if !ok || writer.filePrefix != ProcessRoleGateway {
		t.Fatalf("expected log writer prefix %q, got %T %+v", ProcessRoleGateway, log.Writer(), writer)
	}
	writer.stdout = &bytes.Buffer{}
	if _, err := writer.Write([]byte("gateway log\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	path := filepath.Join(logDir, ProcessRoleGateway+"-"+time.Now().Format("2006-01-02")+".log")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected role-specific log file %q: %v", path, err)
	}
}
