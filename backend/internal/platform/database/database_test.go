package database

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestTranslateErrorPreservesIdentity(t *testing.T) {
	cause := errors.New("duplicate")
	got := TranslateError(cause)
	if !errors.Is(got, cause) || !IsInternal(got) {
		t.Fatal("cause was not preserved")
	}
}

func TestTranslateContextErrors(t *testing.T) {
	if !IsCanceled(TranslateError(context.Canceled)) {
		t.Fatal("cancel")
	}
	if !IsDeadline(TranslateError(context.DeadlineExceeded)) {
		t.Fatal("deadline")
	}
}

func TestGORMLoggerRedactsSQLArgumentsAndDSN(t *testing.T) {
	var b strings.Builder
	l := NewGORMLogger(slog.New(slog.NewTextHandler(&b, nil)))
	l.Trace(context.Background(), time.Now(), func() (string, int64) { return "postgres://user:secret/db SELECT $1", 1 }, nil)
	if strings.Contains(b.String(), "secret") || strings.Contains(b.String(), "$1") {
		t.Fatal("sensitive SQL leaked")
	}
}

func TestOptionsRejectMissingDSN(t *testing.T) {
	if _, err := New(context.Background(), Options{}); err == nil {
		t.Fatal("missing DSN")
	}
}
