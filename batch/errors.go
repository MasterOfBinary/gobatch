package batch

import (
	"errors"
	"fmt"
)

// ErrNilSource is returned by Batch.Go when the provided Source is nil.
// Use errors.Is to check for it.
var ErrNilSource = errors.New("batch: source cannot be nil")

// ErrBatchUsed is returned by Batch.Go when it is called on a Batch that has
// already been used. A Batch is single-use: create a new one with New to run
// again. Use errors.Is to check for it.
var ErrBatchUsed = errors.New("batch: Batch is single-use; create a new Batch with New to run again")

// ProcessorError is returned when a processor fails. It wraps the original
// error from the processor to maintain the error chain while providing
// context about the source of the error.
type ProcessorError struct {
	// Err is the underlying error that occurred in the processor.
	Err error
}

// Error implements the error interface, returning a formatted error message
// that includes the wrapped processor error.
func (e ProcessorError) Error() string {
	return fmt.Sprintf("processor error: %v", e.Err)
}

// Unwrap returns the underlying error for compatibility with errors.Is and errors.As.
func (e ProcessorError) Unwrap() error {
	return e.Err
}

// SourceError is returned when a source fails. It wraps the original
// error from the source to maintain the error chain while providing
// context about the source of the error.
type SourceError struct {
	// Err is the underlying error that occurred in the source.
	Err error
}

// Error implements the error interface, returning a formatted error message
// that includes the wrapped source error.
func (e SourceError) Error() string {
	return fmt.Sprintf("source error: %v", e.Err)
}

// Unwrap returns the underlying error for compatibility with errors.Is and errors.As.
func (e SourceError) Unwrap() error {
	return e.Err
}
