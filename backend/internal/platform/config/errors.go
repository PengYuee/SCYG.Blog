package config

import "fmt"

// FileError reports an explicitly requested configuration file failure.
type FileError struct {
	// Err is the underlying filesystem or YAML parsing cause; it must not contain configuration values.
	Err error
	// Path is the explicit configuration file path requested by the caller.
	Path string
}

// Error returns contextual file information without configuration values.
func (e *FileError) Error() string {
	return fmt.Sprintf("read configuration file %q: %v", e.Path, e.Err)
}

// Unwrap exposes the underlying filesystem or parse error.
func (e *FileError) Unwrap() error { return e.Err }

// ValidationError reports the first invalid field in deterministic schema order.
type ValidationError struct {
	// Field is the stable dotted configuration key that failed validation.
	Field string
	// Rule describes the violated constraint without including the raw configuration value.
	Rule string
}

// Error returns a value-free validation description to prevent secret disclosure.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("configuration field %s: %s", e.Field, e.Rule)
}
