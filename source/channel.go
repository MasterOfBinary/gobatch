package source

import "context"

type channelSource struct {
	items <-chan interface{}
}

// Channel creates a Source that reads from items until items is closed.
// The items channel can be buffered or unbuffered.
//
// Note that it will not return until items is closed, even if ctx is
// canceled. Otherwise data could be lost in the pipeline.
func Channel(items <-chan interface{}) Source {
	return &channelSource{
		items: items,
	}
}

// Read reads from items until the input channel is closed.
func (s *channelSource) Read(ctx context.Context, items chan<- interface{}, errs chan<- error) {
	defer close(items)
	defer close(errs)

	for item := range s.items {
		items <- item
	}
}
