package domain

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidValue        = errors.New("content domain: invalid value")
	ErrInvalidTransition   = errors.New("content domain: invalid transition")
	ErrDuplicateTag        = errors.New("content domain: duplicate tag")
	ErrArticleTypeRequired = errors.New("content domain: article type required")
	ErrContentRequired     = errors.New("content domain: content required")
	ErrStaleVersion        = errors.New("content domain: stale version")
	// ErrNoChange identifies a revision equal to current state.
	ErrNoChange = errors.New("content domain: no change")
	// ErrVersionExhausted identifies an aggregate that cannot safely increment.
	ErrVersionExhausted = errors.New("content domain: version exhausted")
	// ErrInvalidClock identifies a nil or zero domain clock value.
	ErrInvalidClock = errors.New("content domain: invalid clock")
	// ErrTimeRegression identifies mutation time earlier than prior domain time.
	ErrTimeRegression = errors.New("content domain: time regression")
	// ErrDeleted identifies an operation attempted on a soft-deleted entity.
	ErrDeleted = errors.New("content domain: deleted")
)

// VersionConflict describes expected and actual aggregate versions.
type VersionConflict struct {
	Expected Version
	Actual   Version
}

func (conflict *VersionConflict) Error() string {
	return fmt.Sprintf("%v: expected %d, actual %d", ErrStaleVersion, conflict.Expected.Uint64(), conflict.Actual.Uint64())
}
func (*VersionConflict) Unwrap() error { return ErrStaleVersion }
