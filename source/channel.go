package source

import (
	"context"
)

// Channel is a Source that reads from an input channel until it's closed.
type Channel struct {
	Input <-chan interface{}
}

// Read reads from the input channel and emits items until it's closed.
func (s *Channel) Read(_ context.Context) (<-chan interface{}, <-chan error) {
	out := make(chan interface{})
	errs := make(chan error)

	go func() {
		defer close(out)
		defer close(errs)
		for item := range s.Input {
			out <- item
		}
	}()

	return out, errs
}
