package database

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// UnitOfWork runs a callback inside one transaction-bound handle.
type UnitOfWork struct{ db *gorm.DB }

// NewUnitOfWork constructs a transaction coordinator.
func NewUnitOfWork(db *Database) (*UnitOfWork, error) {
	if db == nil || db.db == nil {
		return nil, errors.New("database handle is nil")
	}
	return &UnitOfWork{db.db}, nil
}

// WithinTransaction commits success, rolls back errors, and rejects nesting.
func (w *UnitOfWork) WithinTransaction(ctx context.Context, fn func(context.Context, *gorm.DB) error) (err error) {
	if fn == nil {
		return errors.New("transaction callback is nil")
	}
	if ctx == nil {
		return errors.New("transaction context is nil")
	}
	if _, ok := ctx.Value(transactionKey{}).(*gorm.DB); ok {
		return ErrNestedTransaction
	}
	tx := w.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return TranslateError(tx.Error)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback().Error
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback().Error
		}
	}()
	txctx := context.WithValue(ctx, transactionKey{}, tx)
	if err = fn(txctx, tx); err != nil {
		return TranslateError(err)
	}
	if ctx.Err() != nil {
		return TranslateError(ctx.Err())
	}
	if err = tx.Commit().Error; err != nil {
		return TranslateError(err)
	}
	return nil
}

type transactionKey struct{}

// ErrNestedTransaction identifies an explicitly rejected nested transaction.
var ErrNestedTransaction = fmt.Errorf("nested transactions are not supported")
