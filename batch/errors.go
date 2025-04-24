package batch

import "fmt"

// ProcessorError is returned when a processor fails. It wraps the original
// error from the processor to maintain the error chain while providing
// context about the source of the error.
type ProcessorError struct {
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
