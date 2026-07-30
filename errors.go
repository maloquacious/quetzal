// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package quetzal

import (
	"errors"
	"fmt"
)

// Sentinel errors report conditions callers are likely to branch on. Detailed
// errors returned by this package wrap one of these, so use errors.Is rather
// than comparing values directly.
var (
	// ErrInvalidFormat reports data that does not conform to Quetzal or to
	// the IFF container rules Quetzal relies on.
	ErrInvalidFormat = errors.New("quetzal: invalid format")

	// ErrStoryMismatch reports that a save does not belong to the story
	// image supplied by the caller.
	ErrStoryMismatch = errors.New("quetzal: story mismatch")

	// ErrTruncated reports input that ended before a structure was complete.
	ErrTruncated = errors.New("quetzal: truncated data")

	// ErrLimitExceeded reports input whose declared sizes exceed the
	// configured Limits.
	ErrLimitExceeded = errors.New("quetzal: resource limit exceeded")
)

// ChunkError identifies the chunk and file offset at which an error occurred.
// Offset is the position of the chunk's ID within the input stream.
type ChunkError struct {
	ID     ID
	Offset int64

	// Err is the underlying problem, and wraps one of the package sentinels.
	Err error
}

// Error implements the error interface.
func (e *ChunkError) Error() string {
	return fmt.Sprintf("quetzal: chunk %s at offset %d: %s", e.ID, e.Offset, e.Err)
}

// Unwrap returns the underlying error so that errors.Is and errors.As reach
// the sentinel it wraps.
func (e *ChunkError) Unwrap() error { return e.Err }

// detailError carries a specific message while remaining matchable against a
// sentinel. Its message deliberately omits the "quetzal: " prefix so that
// wrapping it, for example in a ChunkError, does not repeat the prefix.
type detailError struct {
	msg      string
	sentinel error
}

// Error implements the error interface.
func (e *detailError) Error() string { return e.msg }

// Unwrap returns the sentinel this error should match under errors.Is.
func (e *detailError) Unwrap() error { return e.sentinel }

// newErr builds a detailed error that matches sentinel under errors.Is.
func newErr(sentinel error, format string, a ...any) error {
	return &detailError{msg: fmt.Sprintf(format, a...), sentinel: sentinel}
}

// prefixed adds the package prefix to an error that is not already reported
// through a type that supplies one.
func prefixed(err error) error {
	return fmt.Errorf("quetzal: %w", err)
}
