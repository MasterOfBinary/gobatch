package batch

// NextItem retrieves the next source item from the input channel of ps, sets
// its data, and returns it. If the input channel is closed, it returns nil.
// NextItem can be used in the source Read function:
//
//    func (s *source) Read(ctx context.Context, ps batch.PipelineStage) {
//      // Read data into myData...
//      items <- batch.NextItem(ps, myData)
//      // ...
//    }
func NextItem(ps PipelineStage, data interface{}) *Item {
	i, ok := <-ps.Input()
	if !ok {
		return nil
	}
	i.Set(data)
	return i
}

// IgnoreErrors starts a goroutine that reads errors from errs but ignores them.
// It can be used with Batch.Go if errors aren't needed. Since the error channel
// is unbuffered, one cannot just throw away the error channel like this:
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
