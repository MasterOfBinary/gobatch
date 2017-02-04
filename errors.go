package gobatch

// IgnoreErrors starts a goroutine that reads errors from errs but ignores them.
// It can be used with Batch.Go if errors aren't needed. Since the error channel
// is non-buffered, one cannot just throw away the error channel like this:
//
//    // NOTE: bad - this can cause a deadlock!
//    _ = batch.Go(ctx, p, s)
//
// Instead, IgnoreErrors can be used to safely throw away all errors:
//
//    IgnoreErrors(batch.Go(ctx, p, s))
func IgnoreErrors(errs <-chan error) {
	// nil channels always block, so check for nil first to avoid a goroutine
	// leak
	if errs != nil {
		go func() {
			for _ = range errs {
			}
		}()
	}
}

// BatchError is a wrapped error message returned on the channel
type BatchError interface {
	// Original returns the original (unwrapped) error.
	Original() error
}

// ProcessorError is an error returned from the processor.
type ProcessorError struct {
	err error
}

func newProcessorError(err error) error {
	return &ProcessorError{
		err: err,
	}
}

// Error implements error. It returns the error string of the original
// error.
func (e ProcessorError) Error() string {
	return e.err.Error()
}

// Original implements BatchError. It returns the original error.
func (e ProcessorError) Original() error {
	return e.err
}

// SourceError is an error returned from the source.
type SourceError struct {
	err error
}

func newSourceError(err error) error {
	return &SourceError{
		err: err,
	}
}

// Error implements error. It returns the error string of the original
// error.
func (e SourceError) Error() string {
	return e.err.Error()
}

// Original implements BatchError. It returns the original error.
func (e SourceError) Original() error {
	return e.err
}
