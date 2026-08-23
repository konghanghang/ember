package logging

import (
	"errors"
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
	logDir             = "logs"
	ProcessRoleAPI     = "api"
	ProcessRoleGateway = "gateway"
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
	initOnce            = &sync.Once{}
	logWriter io.Writer = os.Stdout
	initErr   error
)

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
		writer := newDailyFileWriter(logDir, filePrefix)
		logWriter = writer
		log.SetOutput(writer)
		log.SetFlags(log.LstdFlags | log.Lmicroseconds)
		gin.DefaultWriter = writer
		gin.DefaultErrorWriter = writer
		initErr = os.MkdirAll(logDir, 0o755)
	})
	return initErr
}

func Writer() io.Writer {
	return logWriter
}
