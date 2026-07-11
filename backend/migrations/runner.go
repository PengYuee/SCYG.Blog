package migrations

import (
	"database/sql"
	"errors"
	"io/fs"
	"sync"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// CurrentVersion 是当前嵌入迁移要求的唯一就绪版本。
const CurrentVersion uint = 1

// Runner applies the embedded versioned schema and owns its migration resources.
type Runner struct {
	closeErr  error
	migrate   *migrate.Migrate
	closeOnce sync.Once
}

// New constructs a runner from the embedded migration source.
func New(db *sql.DB, databaseName string) (*Runner, error) {
	return NewWithSource(db, databaseName, FS)
}

// NewWithSource constructs a runner from an explicit filesystem source.
func NewWithSource(db *sql.DB, databaseName string, migrationFS fs.FS) (*Runner, error) {
	if db == nil {
		return nil, errors.New("database pool is nil")
	}
	if _, err := fs.ReadDir(migrationFS, "."); err != nil {
		return nil, err
	}
	source, err := iofs.New(migrationFS, ".")
	if err != nil {
		return nil, err
	}
	driver, err := postgres.WithInstance(db, &postgres.Config{DatabaseName: databaseName})
	if err != nil {
		return nil, errors.Join(err, source.Close())
	}
	instance, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		return nil, errors.Join(err, source.Close(), driver.Close())
	}
	return &Runner{migrate: instance}, nil
}

// Close releases the migration source and database driver exactly once.
func (r *Runner) Close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		sourceErr, databaseErr := r.migrate.Close()
		r.closeErr = errors.Join(sourceErr, databaseErr)
	})
	return r.closeErr
}

// Up applies all pending migrations.
func (r *Runner) Up() error {
	err := r.migrate.Up()
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}

// Down rolls back all migrations.
func (r *Runner) Down() error {
	err := r.migrate.Down()
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}

// Version returns current version and dirty state.
func (r *Runner) Version() (uint, bool, error) { return r.migrate.Version() }

// Force sets the migration version after an operator has repaired dirty state.
func (r *Runner) Force(version int) error { return r.migrate.Force(version) }

// Files returns the embedded filesystem for source-contract tests.
func Files() fs.FS { return FS }
