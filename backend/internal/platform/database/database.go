// Package database provides PostgreSQL connection and transaction primitives.
package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Options configures a PostgreSQL connection without process configuration reads.
type Options struct {
	// Logger receives parameter-safe records.
	Logger *slog.Logger
	// DSN is supplied by the composition root.
	DSN string
	// ConnMaxLifetime bounds connection reuse.
	ConnMaxLifetime time.Duration
	// MaxOpenConns limits concurrent connections.
	MaxOpenConns int
	// MaxIdleConns limits idle connections.
	MaxIdleConns int
}

// Database owns a GORM handle and its underlying pool.
type Database struct {
	closeErr  error
	db        *gorm.DB
	pool      *sql.DB
	closeOnce sync.Once
}

// New opens, tunes, and pings PostgreSQL.
func New(ctx context.Context, options Options) (*Database, error) {
	if options.DSN == "" {
		return nil, &OptionsError{Field: "dsn", Rule: "must not be empty"}
	}
	if options.MaxOpenConns < 0 || options.MaxIdleConns < 0 || options.MaxIdleConns > options.MaxOpenConns && options.MaxOpenConns > 0 {
		return nil, &OptionsError{Field: "pool", Rule: "limits are invalid"}
	}
	handle, err := gorm.Open(postgres.Open(options.DSN), &gorm.Config{Logger: NewGORMLogger(options.Logger)})
	if err != nil {
		return nil, TranslateError(err)
	}
	pool, err := handle.DB()
	if err != nil {
		return nil, TranslateError(err)
	}
	pool.SetMaxOpenConns(options.MaxOpenConns)
	pool.SetMaxIdleConns(options.MaxIdleConns)
	pool.SetConnMaxLifetime(options.ConnMaxLifetime)
	if err = pool.PingContext(ctx); err != nil {
		return nil, TranslateError(errors.Join(err, pool.Close()))
	}
	return &Database{db: handle, pool: pool}, nil
}

// GORM returns the owned handle for explicit adapters.
func (database *Database) GORM() *gorm.DB { return database.db }

// SQLDB returns the underlying pool for health checks.
func (database *Database) SQLDB() *sql.DB { return database.pool }

// Close releases the pool exactly once and is safe for repeated calls.
func (database *Database) Close() error {
	if database == nil {
		return nil
	}
	database.closeOnce.Do(func() { database.closeErr = TranslateError(database.pool.Close()) })
	return database.closeErr
}

// OptionsError reports invalid constructor input without exposing secrets.
type OptionsError struct {
	Field string
	Rule  string
}

// Error returns a stable validation message.
func (e *OptionsError) Error() string { return fmt.Sprintf("database option %s: %s", e.Field, e.Rule) }
