package logging

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

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

	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
		initOnce = originalInitOnce
		logWriter = originalLogWriter
		initErr = originalInitErr
		log.SetOutput(originalLogOutput)
		log.SetFlags(originalLogFlags)
		gin.DefaultWriter = originalGinWriter
		gin.DefaultErrorWriter = originalGinErrorWriter
	})

	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	initOnce = &sync.Once{}
	logWriter = os.Stdout
	initErr = nil

	if err := Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if Writer() == os.Stdout {
		t.Fatalf("expected Init to replace default writer")
	}
	if _, ok := Writer().(*dailyFileWriter); !ok {
		t.Fatalf("expected dailyFileWriter, got %T", Writer())
	}
	if _, err := os.Stat(apiLogDir); err != nil {
		t.Fatalf("expected log dir %q to exist: %v", apiLogDir, err)
	}
	if !strings.Contains(log.Writer().(*dailyFileWriter).filePrefix, apiLogName) {
		t.Fatalf("expected log writer prefix %q", apiLogName)
	}
}
