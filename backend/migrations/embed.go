// Package migrations embeds and executes the versioned PostgreSQL schema.
package migrations

import (
	"embed"
	"io/fs"
)

// FS contains the immutable versioned PostgreSQL migrations.
//
//go:embed *.sql
var FS embed.FS

var _ fs.FS = FS
