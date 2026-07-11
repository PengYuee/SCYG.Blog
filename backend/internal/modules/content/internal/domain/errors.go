package domain

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidValue identifies malformed domain input.
	ErrInvalidValue = errors.New("content domain: invalid value")
	// ErrInvalidTransition identifies a forbidden status transition.
	ErrInvalidTransition = errors.New("content domain: invalid transition")
	// ErrDuplicateTag identifies a repeated tag association.
	ErrDuplicateTag = errors.New("content domain: duplicate tag")
	// ErrArticleTypeRequired identifies a missing article type.
	ErrArticleTypeRequired = errors.New("content domain: article type required")
	// ErrContentRequired identifies content required for publishing.
	ErrContentRequired = errors.New("content domain: content required")
	// ErrStaleVersion identifies optimistic-concurrency failure.
	ErrStaleVersion = errors.New("content domain: stale version")
)

// VersionConflict describes expected and actual aggregate versions.
type VersionConflict struct {
	Expected Version
	Actual   Version
}

// Error renders deterministic concurrency detail.
func (conflict *VersionConflict) Error() string {
	return fmt.Sprintf("%v: expected %d, actual %d", ErrStaleVersion, conflict.Expected.Uint64(), conflict.Actual.Uint64())
}

// Unwrap supports errors.Is with ErrStaleVersion.
func (*VersionConflict) Unwrap() error { return ErrStaleVersion }
