// Package id generates and validates Forest node identifiers.
//
// IDs are UUIDv7: time-ordered, globally unique, and lexicographically
// sortable by creation time. The string form is the standard lowercase
// hyphenated UUID representation.
package id

import "github.com/google/uuid"

// New returns a new UUIDv7 string.
func New() string {
	return uuid.Must(uuid.NewV7()).String()
}

// IsValid reports whether s is a valid UUID string.
func IsValid(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
