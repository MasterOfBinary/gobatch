package batch

// Error is a wrapped error message returned on the error channel.
type Error interface {
	// Original returns the original (unwrapped) error.
	Original() error
}

// ProcessorError is an error returned from the processor.
type ProcessorError struct {
	err error
}

// Error implements error. It returns the error string of the original
// error.
func (e ProcessorError) Error() string {
	return e.err.Error()
}

// Original implements Error. It returns the original error.
func (e ProcessorError) Original() error {
	return e.err
}

// SourceError is an error returned from the source.
type SourceError struct {
	err error
}

// Error implements error. It returns the error string of the original
// error.
func (e SourceError) Error() string {
	return e.err.Error()
}

// Original implements Error. It returns the original error.
func (e SourceError) Original() error {
	return e.err
}
