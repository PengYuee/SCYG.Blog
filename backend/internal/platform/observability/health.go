package observability

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
)

// ErrShuttingDown indicates readiness was deliberately withdrawn.
var ErrShuttingDown = errors.New("readiness withdrawn for shutdown")

// Probe checks one required readiness dependency.
type Probe func(context.Context) error

// Health distinguishes process liveness from dependency readiness.
type Health struct {
	database  Probe
	migration Probe
	accepting atomic.Bool
}

// NewHealth validates and constructs PostgreSQL and migration readiness probes.
func NewHealth(database, migration Probe) (*Health, error) {
	if database == nil {
		return nil, &OptionsError{field: "database_probe", rule: "must not be nil"}
	}
	if migration == nil {
		return nil, &OptionsError{field: "migration_probe", rule: "must not be nil"}
	}
	return &Health{database: database, migration: migration}, nil
}

// Live reports process liveness only and remains true while code can answer.
func (*Health) Live() bool { return true }

// Ready checks cancellation, shutdown, database, then migration in deterministic order.
func (health *Health) Ready(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !health.accepting.Load() {
		return false, ErrShuttingDown
	}
	if err := health.database(ctx); err != nil {
		return false, fmt.Errorf("database readiness: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := health.migration(ctx); err != nil {
		return false, fmt.Errorf("migration readiness: %w", err)
	}
	return true, nil
}

// Activate 在数据库连通且迁移状态有效后原子开放 readiness。
func (health *Health) Activate() { health.accepting.Store(true) }

// Withdraw atomically revokes readiness before shutdown begins.
func (health *Health) Withdraw() { health.accepting.Store(false) }
