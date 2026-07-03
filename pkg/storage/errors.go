package storage

import "errors"

var (
	// ErrNotFound is returned when a blob or metadata record does not exist.
	ErrNotFound = errors.New("storage: not found")
	// ErrInvalidDigest is returned when a digest fails validation.
	ErrInvalidDigest = errors.New("storage: invalid digest")
)
