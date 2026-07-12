// Package postgres adapts catalog persistence.
package postgres

import "example.com/architecture-valid/internal/modules/catalog/internal/application"

// Reader is a concrete persistence adapter fixture.
type Reader struct{ Port application.ArticleReader }
