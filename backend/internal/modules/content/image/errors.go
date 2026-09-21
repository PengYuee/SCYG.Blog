package image

import (
	"context"
	"errors"
	"fmt"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/database"
)

// Code identifies a stable image application error category.
type Code string

// Stable image error categories.
const (
	CodeValidation       Code = "validation"
	CodePermissionDenied Code = "permission_denied"
	CodeNotFound         Code = "not_found"
	CodeConflict         Code = "conflict"
	CodeInternal         Code = "internal"
)

// Image persistence, validation, and lifecycle sentinels.
var (
	ErrNotFound          = errors.New("image: not found")
	ErrConflict          = errors.New("image: conflict")
	ErrPersistence       = errors.New("image: persistence failure")
	ErrInvalidReference  = errors.New("image: invalid reference")
	ErrInvalidValue      = errors.New("内容领域：值不合法")
	ErrInvalidTransition = errors.New("内容领域：状态迁移不合法")
	ErrTimeRegression    = errors.New("内容领域：时间倒退")
)

// Error is the structured image application error.
type Error struct {
	Code  Code
	Cause error
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

// StableCode returns the consumer-facing image error category.
func (e *Error) StableCode() string {
	if e == nil {
		return string(CodeInternal)
	}
	return string(e.Code)
}

func invalid(field string) error { return fmt.Errorf("%s: %w", field, ErrInvalidValue) }
func validation(err error) error { return &Error{Code: CodeValidation, Cause: err} }
func permission() error {
	return &Error{Code: CodePermissionDenied, Cause: content.ErrPermissionDenied}
}
func notFound() error { return &Error{Code: CodeNotFound, Cause: ErrNotFound} }
func conflict() error { return &Error{Code: CodeConflict, Cause: ErrConflict} }
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
	if errors.Is(err, ErrNotFound) || database.IsNotFound(err) {
		return notFound()
	}
	if errors.Is(err, content.ErrPermissionDenied) {
		return permission()
	}
	if errors.Is(err, ErrInvalidTransition) {
		return conflict()
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return internal(err)
}
