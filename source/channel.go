package source

import (
	"context"

	"github.com/MasterOfBinary/gobatch/batch"
)

type channelSource struct {
	items <-chan interface{}
}

// Channel returns a Source that reads from items until items is closed.
// The items channel can be buffered or unbuffered.
//
// Note that it will not return until items is closed, even if ctx is
// canceled. Otherwise data could be lost in the pipeline.
func Channel(items <-chan interface{}) batch.Source {
	return &channelSource{
		items: items,
	}
}

// Read reads from items until the input channel is closed.
func (s *channelSource) Read(ctx context.Context, in <-chan *batch.Item, items chan<- *batch.Item, errs chan<- error) {
	defer close(items)
	defer close(errs)

	for item := range s.items {
		items <- batch.NextItem(in, item)
	}
}
