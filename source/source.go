package source

import (
	"context"

	"github.com/MasterOfBinary/gobatch/item"
)

// Source reads items that are to be batch processed.
type Source interface {
	// Read reads items and writes them to the items channel. Any errors
	// it encounters while reading are written to the errs channel.
	//
	// Once reading is finished (or when the program ends) both items and
	// errs need to be closed. This signals to Batch that it should drain
	// the pipeline and finish. It is not enough for Read to return.
	//
	//    func (s source) Read(ctx context.Context, items chan<- Item, errs chan<- error) {
	//      defer close(items)
	//      defer close(errs)
	//      // Read items until done...
	//    }
	//
	// Read should not modify an item after adding it to items.
	Read(ctx context.Context, source <-chan item.Item, items chan<- item.Item, errs chan<- error)
}
