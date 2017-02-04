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

type BatchError interface {
	Original() error
}

type ProcessorError struct {
	err error
}

func newProcessorError(err error) error {
	return &ProcessorError{
		err: err,
	}
}

func (e ProcessorError) Error() string {
	return e.err.Error()
}

func (e ProcessorError) Original() error {
	return e.err
}

type SourceError struct {
	err error
}

func newSourceError(err error) error {
	return &SourceError{
		err: err,
	}
}

func (e SourceError) Error() string {
	return e.err.Error()
}

func (e SourceError) Original() error {
	return e.err
}
