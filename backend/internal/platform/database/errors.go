package database

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// Code identifies a stable database failure category.
type Code string

// Stable database error codes.
const (
	CodeUnique      Code = "unique"
	CodeForeignKey  Code = "foreign_key"
	CodeNotFound    Code = "not_found"
	CodeCanceled    Code = "canceled"
	CodeDeadline    Code = "deadline"
	CodeUnavailable Code = "unavailable"
	CodeInternal    Code = "internal"
)

// Error is a classified failure retaining its cause for errors.Is/As.
type Error struct {
	Err  error
	Code Code
}

func (e *Error) Error() string { return fmt.Sprintf("database %s: %v", e.Code, e.Err) }
func (e *Error) Unwrap() error { return e.Err }

// TranslateError maps driver, GORM, and context failures without SQL or DSN output.
func TranslateError(cause error) error {
	if cause == nil {
		return nil
	}
	if errors.Is(cause, context.Canceled) {
		return &Error{Err: cause, Code: CodeCanceled}
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		return &Error{Err: cause, Code: CodeDeadline}
	}
	if errors.Is(cause, gorm.ErrRecordNotFound) {
		return &Error{Err: cause, Code: CodeNotFound}
	}
	var pg *pgconn.PgError
	if errors.As(cause, &pg) {
		switch pg.Code {
		case "23505":
			return &Error{Err: cause, Code: CodeUnique}
		case "23503":
			return &Error{Err: cause, Code: CodeForeignKey}
		case "08000", "08003", "08006", "57P01":
			return &Error{Err: cause, Code: CodeUnavailable}
		}
	}
	if strings.Contains(strings.ToLower(cause.Error()), "connection refused") {
		return &Error{Err: cause, Code: CodeUnavailable}
	}
	return &Error{Err: cause, Code: CodeInternal}
}

func hasCode(err error, code Code) bool {
	var classified *Error
	return errors.As(err, &classified) && classified.Code == code
}

// IsUnique reports a unique conflict.
func IsUnique(err error) bool { return hasCode(err, CodeUnique) }

// IsForeignKey reports a foreign-key conflict.
func IsForeignKey(err error) bool { return hasCode(err, CodeForeignKey) }

// IsNotFound reports an absent row.
func IsNotFound(err error) bool { return hasCode(err, CodeNotFound) }

// IsCanceled reports cancellation.
func IsCanceled(err error) bool { return hasCode(err, CodeCanceled) }

// IsDeadline reports deadline expiry.
func IsDeadline(err error) bool { return hasCode(err, CodeDeadline) }

// IsUnavailable reports an unavailable database.
func IsUnavailable(err error) bool { return hasCode(err, CodeUnavailable) }

// IsInternal reports an unclassified failure.
func IsInternal(err error) bool { return hasCode(err, CodeInternal) }
