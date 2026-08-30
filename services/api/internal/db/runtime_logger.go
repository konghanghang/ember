package db

import (
	"context"
	"io"
	"log"
	"time"

	logpkg "github.com/konghang/ember/backend/internal/logging"
	"gorm.io/gorm/logger"
)

// runtimeGORMLogger delegates to a Warn or Info logger at each event so a
// database-backed LOG_LEVEL update affects SQL diagnostics without rebuilding
// the shared GORM connection.
type runtimeGORMLogger struct {
	info  logger.Interface
	debug logger.Interface
}

// newGORMLogger preserves SQL timing and structure while suppressing all bound
// values at both runtime levels.
func newGORMLogger(output io.Writer) logger.Interface {
	writer := log.New(output, "\r\n", log.LstdFlags)
	baseConfig := logger.Config{
		SlowThreshold:             time.Second,
		IgnoreRecordNotFoundError: false,
		ParameterizedQueries:      true,
		Colorful:                  true,
	}
	infoConfig := baseConfig
	infoConfig.LogLevel = logger.Warn
	debugConfig := baseConfig
	debugConfig.LogLevel = logger.Info
	return &runtimeGORMLogger{
		info:  logger.New(writer, infoConfig),
		debug: logger.New(writer, debugConfig),
	}
}

// LogMode preserves GORM's requested ceiling while retaining the project
// runtime level as the final selector.
func (runtimeLogger *runtimeGORMLogger) LogMode(level logger.LogLevel) logger.Interface {
	infoLevel := level
	if infoLevel > logger.Warn {
		infoLevel = logger.Warn
	}
	return &runtimeGORMLogger{
		info:  runtimeLogger.info.LogMode(infoLevel),
		debug: runtimeLogger.debug.LogMode(level),
	}
}

// Info emits normal SQL diagnostics only while system Debug is active.
func (runtimeLogger *runtimeGORMLogger) Info(ctx context.Context, message string, args ...interface{}) {
	if logpkg.DebugEnabled() {
		runtimeLogger.debug.Info(ctx, message, args...)
	}
}

// Warn keeps GORM warnings visible at either project level.
func (runtimeLogger *runtimeGORMLogger) Warn(ctx context.Context, message string, args ...interface{}) {
	runtimeLogger.active().Warn(ctx, message, args...)
}

// Error keeps GORM errors visible at either project level.
func (runtimeLogger *runtimeGORMLogger) Error(ctx context.Context, message string, args ...interface{}) {
	runtimeLogger.active().Error(ctx, message, args...)
}

// Trace selects the current runtime level for every SQL event.
func (runtimeLogger *runtimeGORMLogger) Trace(
	ctx context.Context,
	begin time.Time,
	query func() (string, int64),
	err error,
) {
	runtimeLogger.active().Trace(ctx, begin, query, err)
}

// ParamsFilter guarantees that bound values never leave GORM regardless of
// which underlying level is active.
func (runtimeLogger *runtimeGORMLogger) ParamsFilter(
	_ context.Context,
	sql string,
	_ ...interface{},
) (string, []interface{}) {
	return sql, nil
}

func (runtimeLogger *runtimeGORMLogger) active() logger.Interface {
	if logpkg.DebugEnabled() {
		return runtimeLogger.debug
	}
	return runtimeLogger.info
}
