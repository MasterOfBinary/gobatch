package source

import "context"

// Source reads items that are to be batch processed.
type Source interface {
	// Read reads items and writes them to the items channel. Any errors
	// it encounters while reading are written to the errs channel.
	//
	// Once reading is finished (or when the program ends) both items and
	// errs need to be closed. This signals to Batch that it should drain
	// the pipeline and finish. It is not enough for Read to return.
	//
	//    func (s source) Read(ctx context.Context, items chan<- interface{}, errs chan<- error) {
	//      defer close(items)
	//      defer close(errs)
	//      // Read items here...
	//    }
	Read(ctx context.Context, items chan<- interface{}, errs chan<- error)
}
