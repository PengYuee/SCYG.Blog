package article

import (
	"errors"
	"fmt"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/database"
)

// Code identifies a stable article application error category.
type Code string

// Stable article error categories.
const (
	CodeValidation         Code = "validation"
	CodePermissionDenied   Code = "permission_denied"
	CodeNotFound           Code = "not_found"
	CodeAlreadyExists      Code = "already_exists"
	CodeFailedPrecondition Code = "failed_precondition"
	CodeStale              Code = "stale_version"
	CodeInternal           Code = "internal"
)

// Persistence and lookup sentinels returned by the article feature.
var (
	ErrNotFound           = errors.New("article: not found")
	ErrConflict           = errors.New("article: conflict")
	ErrFailedPrecondition = errors.New("article: failed precondition")
	ErrPersistence        = errors.New("article: persistence failure")
)

// Domain validation and lifecycle sentinels returned by article operations.
var (
	ErrInvalidValue        = errors.New("内容领域：值不合法")
	ErrInvalidTransition   = errors.New("内容领域：状态迁移不合法")
	ErrDuplicateTag        = errors.New("内容领域：标签重复")
	ErrArticleTypeRequired = errors.New("内容领域：必须指定文章分类")
	ErrContentRequired     = errors.New("内容领域：必须提供正文")
	ErrNoChange            = errors.New("内容领域：没有变更")
	ErrVersionExhausted    = errors.New("内容领域：版本号已耗尽")
	ErrInvalidClock        = errors.New("内容领域：时钟无效")
	ErrTimeRegression      = errors.New("内容领域：时间倒退")
	ErrDeleted             = errors.New("内容领域：实体已删除")
)

// VersionConflict describes an optimistic-concurrency mismatch.
type VersionConflict struct {
	Expected Version
	Actual   Version
}

func (conflict *VersionConflict) Error() string {
	return fmt.Sprintf("%v：期望版本 %d，实际版本 %d", ErrStaleVersion, conflict.Expected.Uint64(), conflict.Actual.Uint64())
}
func (*VersionConflict) Unwrap() error { return ErrStaleVersion }
func invalid(field string) error       { return fmt.Errorf("%s: %w", field, ErrInvalidValue) }

// Error is the structured article application error.
type Error struct {
	Code             Code
	Cause            error
	Expected, Actual uint64
}

func (e *Error) Error() string {
	if e == nil {
		return string(CodeInternal)
	}
	if e.Cause == nil {
		return string(e.Code)
	}
	return fmt.Sprintf("%s: %v", e.Code, e.Cause)
}
func (e *Error) Unwrap() error { return e.Cause }

// StableCode returns the consumer-facing article error category.
func (e *Error) StableCode() string {
	if e == nil {
		return string(CodeInternal)
	}
	return string(e.Code)
}
func validation(err error) error { return &Error{Code: CodeValidation, Cause: err} }
func permission() error {
	return &Error{Code: CodePermissionDenied, Cause: content.ErrPermissionDenied}
}
func notFound() error      { return &Error{Code: CodeNotFound, Cause: ErrNotFound} }
func alreadyExists() error { return &Error{Code: CodeAlreadyExists, Cause: ErrConflict} }
func precondition() error {
	return &Error{Code: CodeFailedPrecondition, Cause: ErrFailedPrecondition}
}

func stale(expected, actual uint64) error {
	return &Error{Code: CodeStale, Cause: ErrStaleVersion, Expected: expected, Actual: actual}
}

// ErrStaleVersion identifies an optimistic-concurrency conflict.
var ErrStaleVersion = errors.New("article: stale version")

func internal(err error) error {
	return &Error{Code: CodeInternal, Cause: errors.Join(ErrPersistence, err)}
}

func stable(err error) error {
	if err == nil {
		return nil
	}
	var known *Error
	if errors.As(err, &known) {
		return known
	}
	var versionConflict *VersionConflict
	if errors.As(err, &versionConflict) {
		return stale(versionConflict.Expected.Uint64(), versionConflict.Actual.Uint64())
	}
	switch {
	case errors.Is(err, ErrNotFound) || database.IsNotFound(err):
		return notFound()
	case errors.Is(err, ErrConflict):
		return alreadyExists()
	case errors.Is(err, ErrFailedPrecondition), errors.Is(err, ErrInvalidTransition), errors.Is(err, ErrDeleted), errors.Is(err, ErrNoChange):
		return precondition()
	case errors.Is(err, ErrStaleVersion):
		return stale(0, 0)
	case errors.Is(err, ErrInvalidValue), errors.Is(err, ErrDuplicateTag), errors.Is(err, ErrArticleTypeRequired), errors.Is(err, ErrContentRequired):
		return validation(err)
	default:
		return internal(err)
	}
}

func translate(err error) error {
	if err == nil {
		return nil
	}
	classified := database.TranslateError(err)
	switch {
	case database.IsCanceled(classified):
		return errContextCanceled
	case database.IsDeadline(classified):
		return errContextDeadline
	case database.IsUnique(classified):
		return alreadyExists()
	case database.IsForeignKey(classified):
		return precondition()
	default:
		return internal(classified)
	}
}

var (
	errContextCanceled = errors.New("context canceled")
	errContextDeadline = errors.New("context deadline exceeded")
)
