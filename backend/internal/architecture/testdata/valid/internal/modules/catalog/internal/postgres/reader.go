// Package postgres adapts catalog persistence.
package postgres

import "github.com/PengYuee/SCYG.Blog/backend/internal/modules/catalog/internal/application"

// Reader is a concrete persistence adapter fixture.
type Reader struct{ Port application.ArticleReader }
