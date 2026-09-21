package taxonomy

import (
	"errors"
	"fmt"
)

// Code identifies a stable taxonomy application error category.
type Code string

// Stable taxonomy error categories.
const (
	CodeValidation         Code = "validation"
	CodePermissionDenied   Code = "permission_denied"
	CodeNotFound           Code = "not_found"
	CodeAlreadyExists      Code = "already_exists"
	CodeFailedPrecondition Code = "failed_precondition"
	CodeStaleVersion       Code = "stale_version"
	CodeInternal           Code = "internal"
)

// Taxonomy validation, lookup, lifecycle, and persistence sentinels.
var (
	ErrNotFound           = errors.New("taxonomy: resource not found")
	ErrAlreadyExists      = errors.New("taxonomy: resource already exists")
	ErrFailedPrecondition = errors.New("taxonomy: failed precondition")
	ErrStaleVersion       = errors.New("taxonomy: stale version")
	ErrDeleted            = errors.New("taxonomy: resource is deleted")
	ErrNoChange           = errors.New("taxonomy: no change")
	ErrInvalidData        = errors.New("taxonomy: invalid persisted data")
	ErrPersistence        = errors.New("taxonomy: persistence failure")
)

// Error is the structured taxonomy application error.
type Error struct {
	Code     Code
	Cause    error
	Expected uint64
	Actual   uint64
}

func (failure *Error) Error() string {
	if failure == nil {
		return string(CodeInternal)
	}
	if failure.Expected != 0 || failure.Actual != 0 {
		return fmt.Sprintf("%s: expected=%d actual=%d", failure.Code, failure.Expected, failure.Actual)
	}
	if failure.Cause == nil {
		return string(failure.Code)
	}
	return fmt.Sprintf("%s: %v", failure.Code, failure.Cause)
}
func (failure *Error) Unwrap() error { return failure.Cause }

// StableCode returns the consumer-facing taxonomy error category.
func (failure *Error) StableCode() string {
	if failure == nil {
		return string(CodeInternal)
	}
	return string(failure.Code)
}

func invalid(field string) error {
	return &Error{Code: CodeValidation, Cause: fmt.Errorf("%s is invalid", field)}
}
func permission() error { return &Error{Code: CodePermissionDenied, Cause: ErrPermissionDenied} }
func notFound(kind string) error {
	return &Error{Code: CodeNotFound, Cause: fmt.Errorf("%s: %w", kind, ErrNotFound)}
}
func conflict() error     { return &Error{Code: CodeAlreadyExists, Cause: ErrAlreadyExists} }
func precondition() error { return &Error{Code: CodeFailedPrecondition, Cause: ErrFailedPrecondition} }

func stale(expected, actual uint64) error {
	return &Error{Code: CodeStaleVersion, Cause: ErrStaleVersion, Expected: expected, Actual: actual}
}
func internal() error { return &Error{Code: CodeInternal, Cause: ErrPersistence} }

// ErrPermissionDenied indicates that the taxonomy action is not authorized.
var ErrPermissionDenied = errors.New("taxonomy: permission denied")

func stable(err error) error {
	if err == nil {
		return nil
	}
	var known *Error
	if errors.As(err, &known) {
		return known
	}
	if errors.Is(err, ErrNotFound) {
		return notFound("resource")
	}
	if errors.Is(err, ErrAlreadyExists) {
		return conflict()
	}
	if errors.Is(err, ErrFailedPrecondition) || errors.Is(err, ErrDeleted) || errors.Is(err, ErrNoChange) {
		return precondition()
	}
	if errors.Is(err, ErrStaleVersion) {
		return stale(0, 0)
	}
	if errors.Is(err, ErrInvalidData) {
		return internal()
	}
	return internal()
}
