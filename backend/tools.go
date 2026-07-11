//go:build tools

// Package tools documents the exact tool isolation contract for this module.
// Taskfile.yml runs each command with "go run package@version", which makes
// Go ignore this module's go.mod and keeps tool implementations out of its graph.
package tools
