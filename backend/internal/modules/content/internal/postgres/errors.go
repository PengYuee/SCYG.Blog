package postgres

import (
	"errors"
	"fmt"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/internal/domain"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/database"
)

func notFound(kind string) error {
	return &content.ApplicationError{Code: content.CodeNotFound, Kind: content.KindMissing, Cause: fmt.Errorf("%s not found", kind)}
}
func conflict(cause error) error {
	return &content.ApplicationError{Code: content.CodeAlreadyExists, Kind: content.KindConflict, Cause: cause}
}
func failedPrecondition(cause error) error {
	return &content.ApplicationError{Code: content.CodeFailedPrecondition, Kind: content.KindConflict, Cause: cause}
}
func stale(expected, actual uint64) error {
	return &content.ApplicationError{Code: content.CodeStaleVersion, Kind: content.KindConflict, Cause: domain.ErrStaleVersion, ExpectedVersion: expected, ActualVersion: actual}
}
func translate(err error) error {
	if err == nil {
		return nil
	}
	var applicationError *content.ApplicationError
	if errors.As(err, &applicationError) {
		return err
	}
	translated := database.TranslateError(err)
	switch {
	case database.IsUnique(translated):
		return conflict(translated)
	case database.IsForeignKey(translated):
		return failedPrecondition(translated)
	default:
		return translated
	}
}
