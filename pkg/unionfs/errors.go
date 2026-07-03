package unionfs

import "errors"

var (
	// ErrNotFound is returned when a path is not present in the union view.
	ErrNotFound = errors.New("unionfs: not found")
	// ErrNotMounted is returned when operating on an unmounted handle.
	ErrNotMounted = errors.New("unionfs: not mounted")
	// ErrInvalidPath is returned when a relative path is unsafe or malformed.
	ErrInvalidPath = errors.New("unionfs: invalid path")
	// ErrAlreadyMounted is returned when a mount identifier is already active.
	ErrAlreadyMounted = errors.New("unionfs: already mounted")
)
