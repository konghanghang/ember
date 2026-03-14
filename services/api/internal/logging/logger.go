package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	apiLogDir  = "logs"
	apiLogName = "app"
)

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
	initOnce  sync.Once
	logWriter io.Writer = os.Stdout
	initErr   error
)

func Init() error {
	initOnce.Do(func() {
		writer := newDailyFileWriter(apiLogDir, apiLogName)
		logWriter = writer
		log.SetOutput(writer)
		log.SetFlags(log.LstdFlags | log.Lmicroseconds)
		gin.DefaultWriter = writer
		gin.DefaultErrorWriter = writer
		initErr = os.MkdirAll(apiLogDir, 0o755)
	})
	return initErr
}

func Writer() io.Writer {
	return logWriter
}
