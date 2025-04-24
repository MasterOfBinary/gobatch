package batch

import "fmt"

// ProcessorError is returned when a processor fails.
type ProcessorError struct {
	Err error
}

func (e ProcessorError) Error() string {
	return fmt.Sprintf("processor error: %v", e.Err)
}

func (e ProcessorError) Unwrap() error {
	return e.Err
}

// SourceError is returned when a source fails.
type SourceError struct {
	Err error
}

func (e SourceError) Error() string {
	return fmt.Sprintf("source error: %v", e.Err)
}

func (e SourceError) Unwrap() error {
	return e.Err
}
