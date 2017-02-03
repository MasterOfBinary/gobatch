package source

import "context"

type channelSource struct {
	items <-chan interface{}
}

func Channel(items <-chan interface{}) Source {
	return &channelSource{
		items: items,
	}
}

func (s *channelSource) Read(ctx context.Context, items chan<- interface{}, errs chan<- error) {
	for item := range s.items {
		items <- item
	}

	close(items)
	close(errs)
}
