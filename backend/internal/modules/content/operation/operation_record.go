// Package operation owns the global Blog write-operation success ledger.
package operation

import "time"

// record stores a reservation or a completed operation inside its business transaction.
type record struct {
	OperationID string     `gorm:"column:operation_id;primaryKey"`
	ArticleID   *int64     `gorm:"column:article_id"`
	SucceededAt *time.Time `gorm:"column:succeeded_at"`
	ExpiresAt   *time.Time `gorm:"column:expires_at"`
}

func (record) TableName() string { return "article_operations" }
