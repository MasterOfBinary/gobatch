package source

import (
	"context"

	"github.com/MasterOfBinary/gobatch/item"
)

type errorSource struct {
	err error
}

// Error returns a Source that returns an error and then closes immediately.
// It can be used as a mock Source.
func Error(err error) Source {
	return &errorSource{
		err: err,
	}
}

// Read returns an error and then closes.
func (s *errorSource) Read(ctx context.Context, source <-chan item.Item, items chan<- item.Item, errs chan<- error) {
	errs <- s.err
	close(items)
	close(errs)
}
