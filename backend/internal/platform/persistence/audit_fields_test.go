package persistence

import (
	"errors"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm/schema"
)

func TestNewAuditFields_NormalizesTimesAndDerivesDeleteState(t *testing.T) {
	location := time.FixedZone("test", 8*60*60)
	createdAt := time.Date(2026, 7, 12, 12, 0, 0, 0, location)
	updatedAt := createdAt.Add(time.Hour)
	deletedAt := updatedAt.Add(time.Hour)

	fields := NewAuditFields(createdAt, updatedAt, deletedAt)
	if !fields.CreatedAt.Equal(createdAt.UTC()) || fields.CreatedAt.Location() != time.UTC {
		t.Fatalf("created_at = %v, want UTC %v", fields.CreatedAt, createdAt.UTC())
	}
	if fields.UpdatedAt == nil || !fields.UpdatedAt.Equal(updatedAt.UTC()) {
		t.Fatalf("updated_at = %v, want %v", fields.UpdatedAt, updatedAt.UTC())
	}
	if fields.DeletedAt == nil || !fields.DeletedAt.Equal(deletedAt.UTC()) {
		t.Fatalf("deleted_at = %v, want %v", fields.DeletedAt, deletedAt.UTC())
	}
	if !fields.IsDeleted {
		t.Fatal("is_deleted = false, want true")
	}
	if err := fields.Validate(); err != nil {
		t.Fatalf("validate audit fields: %v", err)
	}
	active := NewAuditFields(createdAt, time.Time{}, time.Time{})
	if active.UpdatedAt != nil || active.DeletedAt != nil || active.IsDeleted {
		t.Fatalf("active audit fields = %#v, want nullable timestamps and active state", active)
	}
	if err := active.Validate(); err != nil {
		t.Fatalf("validate active audit fields: %v", err)
	}
}

func TestAuditFields_ValidateRejectsDeleteMismatch(t *testing.T) {
	createdAt := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	deletedAt := createdAt.Add(time.Hour)
	fields := AuditFields{CreatedAt: createdAt, DeletedAt: &deletedAt}

	if err := fields.Validate(); !errors.Is(err, ErrInvalidAuditFields) {
		t.Fatalf("error = %v, want ErrInvalidAuditFields", err)
	}
}

func TestAuditFields_EffectiveUpdatedAtFallsBackForLegacyRows(t *testing.T) {
	createdAt := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	fields := AuditFields{CreatedAt: createdAt}
	if got := fields.EffectiveUpdatedAt(); !got.Equal(createdAt) {
		t.Fatalf("effective updated_at = %v, want %v", got, createdAt)
	}

	updatedAt := createdAt.Add(time.Hour)
	if got := EffectiveUpdatedAt(createdAt, &updatedAt); !got.Equal(updatedAt) {
		t.Fatalf("effective updated_at = %v, want %v", got, updatedAt)
	}
}

func TestAuditFields_DoNotEnableGORMAutomaticTimestamps(t *testing.T) {
	type record struct {
		ID int `gorm:"column:id;primaryKey"`
		AuditFields
	}

	parsed, err := schema.Parse(&record{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse record schema: %v", err)
	}
	for _, name := range []string{"CreatedAt", "UpdatedAt"} {
		field := parsed.FieldsByName[name]
		if field == nil {
			t.Fatalf("field %s is missing", name)
		}
		if field.AutoCreateTime != 0 || field.AutoUpdateTime != 0 {
			t.Fatalf("field %s enables automatic timestamp mutation", name)
		}
	}
}
