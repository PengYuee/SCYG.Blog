package database

import (
	"context"
	"log/slog"
	"time"

	"gorm.io/gorm/logger"
)

// gormLogger emits metadata only, never SQL, bind arguments, or connection strings.
const (
	rowsKey    = "rows"
	elapsedKey = "elapsed"
)

type gormLogger struct {
	log   *slog.Logger
	level logger.LogLevel
}

// NewGORMLogger returns a parameter-safe GORM logger.
func NewGORMLogger(log *slog.Logger) logger.Interface {
	return &gormLogger{log: log, level: logger.Warn}
}

func (g *gormLogger) LogMode(level logger.LogLevel) logger.Interface {
	return &gormLogger{log: g.log, level: level}
}

func (g *gormLogger) Info(ctx context.Context, _ string, _ ...interface{}) {
	if g.log != nil && g.level >= logger.Info {
		g.log.InfoContext(ctx, "database message")
	}
}

func (g *gormLogger) Warn(ctx context.Context, _ string, _ ...interface{}) {
	if g.log != nil && g.level >= logger.Warn {
		g.log.WarnContext(ctx, "database warning")
	}
}

func (g *gormLogger) Error(ctx context.Context, _ string, _ ...interface{}) {
	if g.log != nil && g.level >= logger.Error {
		g.log.ErrorContext(ctx, "database error")
	}
}

func (g *gormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if g.log == nil {
		return
	}
	_, rows := fc()
	if err != nil && g.level >= logger.Error {
		g.log.ErrorContext(ctx, "database query failed", slog.Int64(rowsKey, rows), slog.Duration(elapsedKey, time.Since(begin)))
		return
	}
	if g.level >= logger.Warn && time.Since(begin) > time.Second {
		g.log.WarnContext(ctx, "database query slow", slog.Int64(rowsKey, rows), slog.Duration(elapsedKey, time.Since(begin)))
	}
}
