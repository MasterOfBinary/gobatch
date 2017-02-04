package processor

import "context"

// Processor processes items in batches.
type Processor interface {
	// Process processes items and returns any errors on the errs channel.
	// When it is done, it must close the errs channel to signify that it's
	// finished processing. Simply returning isn't enough.
	//
	//    func (p *processor) Process(ctx context.Context, items []interface{}, errs chan<- error) {
	//      defer close(errs)
	//      // Do processing here...
	//    }
	//
	// Batch does not wait for Process to finish, so it can spawn a
	// goroutine and then return, as long as errs is closed at the end.
	//
	//    // This is ok
	//    func (p *processor) Process(ctx context.Context, items []interface{}, errs chan<- error) {
	//      go func() {
	//        defer close(errs)
	//        time.Sleep(time.Second)
	//        fmt.Println(items)
	//      }()
	//    }
	Process(ctx context.Context, items []interface{}, errs chan<- error)
}
