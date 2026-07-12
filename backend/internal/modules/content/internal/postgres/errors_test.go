package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/database"
)

func Test_ErrorMapping_unique_and_foreign_key_truncate_adapter_causes(t *testing.T) {
	tests := []struct {
		name         string
		code         string
		wantCode     content.ErrorCode
		wantSentinel error
	}{
		{name: "unique", code: "23505", wantCode: content.CodeAlreadyExists, wantSentinel: content.ErrConflict},
		{name: "foreign key", code: "23503", wantCode: content.CodeFailedPrecondition, wantSentinel: content.ErrFailedPrecondition},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			driver := &pgconn.PgError{Code: test.code, Message: "secret driver detail", ConstraintName: "secret_constraint"}
			// When
			err := translate(fmt.Errorf("wrapped adapter failure: %w", driver))
			// Then
			assertPublicError(t, err, test.wantCode, test.wantSentinel)
			var leakedDriver *pgconn.PgError
			if errors.As(err, &leakedDriver) {
				t.Fatal("pgconn error leaked")
			}
			var leakedDatabase *database.Error
			if errors.As(err, &leakedDatabase) {
				t.Fatal("database error leaked")
			}
		})
	}
}

func Test_ErrorMapping_not_found_stale_and_context_preserve_only_public_semantics(t *testing.T) {
	// Given / When / Then: GORM not found becomes stable module not found.
	assertPublicError(t, translate(gorm.ErrRecordNotFound), content.CodeNotFound, content.ErrNotFound)
	if errors.Is(translate(gorm.ErrRecordNotFound), gorm.ErrRecordNotFound) {
		t.Fatal("GORM sentinel leaked")
	}
	// Stale keeps versions and domain sentinel without adapter causes.
	err := stale(4, 5)
	assertPublicError(t, err, content.CodeStaleVersion, domain.ErrStaleVersion)
	var applicationError *content.ApplicationError
	errors.As(err, &applicationError)
	if applicationError.ExpectedVersion != 4 || applicationError.ActualVersion != 5 {
		t.Fatal("stale versions lost")
	}
	// Standard context sentinels remain observable, database wrappers do not.
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		mapped := translate(database.TranslateError(cause))
		if !errors.Is(mapped, cause) {
			t.Fatalf("context sentinel lost: %v", cause)
		}
		var leaked *database.Error
		if errors.As(mapped, &leaked) {
			t.Fatalf("database error leaked for %v", cause)
		}
	}
}

func Test_ErrorMapping_extracts_existing_application_error_from_database_wrapper(t *testing.T) {
	// Given
	inner := conflict()
	wrapped := database.TranslateError(inner)
	// When
	err := translate(wrapped)
	// Then
	assertPublicError(t, err, content.CodeAlreadyExists, content.ErrConflict)
	var leaked *database.Error
	if errors.As(err, &leaked) {
		t.Fatal("outer database error leaked")
	}
}

func Test_ErrorMapping_extracts_domain_version_conflict_from_database_wrapper(t *testing.T) {
	expected, expectedErr := domain.NewVersion(1)
	actual, actualErr := domain.NewVersion(2)
	if expectedErr != nil || actualErr != nil {
		t.Fatalf("构造版本夹具失败：%v %v", expectedErr, actualErr)
	}
	wrapped := database.TranslateError(&domain.VersionConflict{Expected: expected, Actual: actual})
	err := translate(wrapped)
	assertPublicError(t, err, content.CodeStaleVersion, domain.ErrStaleVersion)
	var applicationError *content.ApplicationError
	if !errors.As(err, &applicationError) || applicationError.ExpectedVersion != 1 || applicationError.ActualVersion != 2 {
		t.Fatalf("stale 版本映射错误：%v", err)
	}
}
func assertPublicError(t *testing.T, err error, wantCode content.ErrorCode, wantSentinel error) {
	t.Helper()
	var applicationError *content.ApplicationError
	if !errors.As(err, &applicationError) {
		t.Fatalf("ApplicationError missing: %v", err)
	}
	if applicationError.Code != wantCode {
		t.Fatalf("code=%s want=%s", applicationError.Code, wantCode)
	}
	if !errors.Is(err, wantSentinel) {
		t.Fatalf("sentinel missing: %v", wantSentinel)
	}
}

func assertNoAdapterCause(t *testing.T, err error) {
	t.Helper()
	var driver *pgconn.PgError
	if errors.As(err, &driver) {
		t.Fatal("pgconn error leaked")
	}
	var classified *database.Error
	if errors.As(err, &classified) {
		t.Fatal("database error leaked")
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal("GORM not-found sentinel leaked")
	}
}
