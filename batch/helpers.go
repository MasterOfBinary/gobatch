package batch

// IgnoreErrors starts a goroutine that reads errors from errs but ignores them.
// It can be used with Batch.Go if errors aren't needed. Since the error channel
// is unbuffered, one cannot just throw away the error channel like this:
//
//	// NOTE: bad - this can cause a deadlock!
//	_ = batch.Go(ctx, p, s)
//
// Instead, IgnoreErrors can be used to safely throw away all errors:
//
//	batch.IgnoreErrors(myBatch.Go(ctx, p, s))
func IgnoreErrors(errs <-chan error) {
	// nil channels always block, so check for nil first to avoid a goroutine
	// leak
	if errs != nil {
		go func() {
			for range errs {
			}
		}()
	}
}
