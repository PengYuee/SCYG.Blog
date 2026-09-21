// Package persistence contains reusable persistence-layer building blocks.
package persistence

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidAuditFields identifies an invalid audit column combination.
var ErrInvalidAuditFields = errors.New("invalid audit fields")

// AuditFields contains the shared audit and soft-delete columns used by records
// that follow the backend persistence contract. It is not a GORM model and does
// not enable GORM's automatic soft-delete behavior.
type AuditFields struct {
	// CreatedAt is the persisted creation timestamp.
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime:false"`
	// UpdatedAt is the nullable persisted modification timestamp.
	UpdatedAt *time.Time `gorm:"column:updated_at;autoUpdateTime:false"`
	// DeletedAt is non-nil when the record is soft-deleted.
	DeletedAt *time.Time `gorm:"column:deleted_at"`
	// IsDeleted mirrors whether DeletedAt is non-nil.
	IsDeleted bool `gorm:"column:is_deleted"`
}

// NewAuditFields converts domain timestamps into the shared persistence shape.
// Zero updated/deleted times are stored as NULL; deletion state is derived from
// the deletion timestamp so the two columns cannot diverge on writes.
func NewAuditFields(createdAt, updatedAt, deletedAt time.Time) AuditFields {
	return AuditFields{
		CreatedAt: createdAt.UTC(),
		UpdatedAt: nullableTime(updatedAt),
		DeletedAt: nullableTime(deletedAt),
		IsDeleted: !deletedAt.IsZero(),
	}
}

// Validate checks the invariant shared by audit-enabled persistence records.
func (fields AuditFields) Validate() error {
	createdAt := fields.CreatedAt.UTC()
	if createdAt.IsZero() {
		return fmt.Errorf("created_at: %w", ErrInvalidAuditFields)
	}
	if fields.UpdatedAt != nil {
		updatedAt := fields.UpdatedAt.UTC()
		if updatedAt.IsZero() || updatedAt.Before(createdAt) {
			return fmt.Errorf("updated_at: %w", ErrInvalidAuditFields)
		}
	}
	if fields.DeletedAt != nil {
		deletedAt := fields.DeletedAt.UTC()
		if deletedAt.IsZero() || deletedAt.Before(createdAt) {
			return fmt.Errorf("deleted_at: %w", ErrInvalidAuditFields)
		}
		if fields.UpdatedAt != nil && deletedAt.Before(fields.UpdatedAt.UTC()) {
			return fmt.Errorf("deleted_at precedes updated_at: %w", ErrInvalidAuditFields)
		}
	}
	if fields.IsDeleted != (fields.DeletedAt != nil) {
		return fmt.Errorf("is_deleted and deleted_at disagree: %w", ErrInvalidAuditFields)
	}
	return nil
}

// EffectiveUpdatedAt returns updatedAt or the creation time when updatedAt is nil.
func EffectiveUpdatedAt(createdAt time.Time, updatedAt *time.Time) time.Time {
	if updatedAt == nil {
		return createdAt.UTC()
	}
	return updatedAt.UTC()
}

// EffectiveUpdatedAt returns the persisted update time, falling back to the
// creation time for legacy rows whose updated_at is NULL.
func (fields AuditFields) EffectiveUpdatedAt() time.Time {
	return EffectiveUpdatedAt(fields.CreatedAt, fields.UpdatedAt)
}

func nullableTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	value = value.UTC()
	return &value
}
