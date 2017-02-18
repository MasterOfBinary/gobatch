package batch

// NextItem retrieves the next source item from the source channel, sets its data,
// and returns it. It can be used in the Source.Read function:
//
//    func (s *source) Read(ctx context.Context, in <-chan *batch.Item, items chan<- *batch.Item, errs chan<- error) {
//      // Read data into myData...
//      items <- batch.NextItem(in, myData)
//      // ...
//    }
func NextItem(ps PipelineStage, data interface{}) *Item {
	i := <-ps.Input()
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
