// Package broken proves the compiler gate rejects unresolved types.
package broken

// Value deliberately references a missing symbol.
type Value struct{ Missing MissingType }
