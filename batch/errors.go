package batch

import "fmt"

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

// ItemError is returned when an individual item has an error after processing.
// Unlike ProcessorError, which indicates a processor-wide failure, ItemError
// represents a failure specific to a single item. The ItemID field identifies
// which item failed, making it easier to debug issues in large batches.
type ItemError struct {
	// ItemID is the unique identifier of the item that failed.
	ItemID uint64

	// Err is the underlying error set on the item.
	Err error
}

// Error implements the error interface, returning a formatted error message
// that includes the item ID and the wrapped error.
func (e *ItemError) Error() string {
	return fmt.Sprintf("item %d error: %v", e.ItemID, e.Err)
}

// Unwrap returns the underlying error for compatibility with errors.Is and errors.As.
func (e *ItemError) Unwrap() error {
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
