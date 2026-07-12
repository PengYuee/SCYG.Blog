package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/database"
)

func notFound(kind string) error {
	return &content.ApplicationError{Code: content.CodeNotFound, Kind: content.KindMissing, Cause: fmt.Errorf("%s: %w", kind, content.ErrNotFound)}
}
func conflict() error {
	return &content.ApplicationError{Code: content.CodeAlreadyExists, Kind: content.KindConflict, Cause: content.ErrConflict}
}
func failedPrecondition() error {
	return &content.ApplicationError{Code: content.CodeFailedPrecondition, Kind: content.KindConflict, Cause: content.ErrFailedPrecondition}
}
func stale(expected, actual uint64) error {
	return &content.ApplicationError{Code: content.CodeStaleVersion, Kind: content.KindConflict, Cause: domain.ErrStaleVersion, ExpectedVersion: expected, ActualVersion: actual}
}
func internalFailure() error {
	return &content.ApplicationError{Code: content.CodeInternal, Kind: content.KindInternal, Cause: content.ErrPersistence}
}
func translate(err error) error {
	if err == nil {
		return nil
	}
	var applicationError *content.ApplicationError
	if errors.As(err, &applicationError) {
		return applicationError
	}
	var versionConflict *domain.VersionConflict
	if errors.As(err, &versionConflict) {
		return stale(versionConflict.Expected.Uint64(), versionConflict.Actual.Uint64())
	}
	translated := database.TranslateError(err)
	switch {
	case database.IsUnique(translated):
		return conflict()
	case database.IsForeignKey(translated):
		return failedPrecondition()
	case database.IsNotFound(translated):
		return notFound("resource")
	case database.IsCanceled(translated):
		return context.Canceled
	case database.IsDeadline(translated):
		return context.DeadlineExceeded
	default:
		return internalFailure()
	}
}
