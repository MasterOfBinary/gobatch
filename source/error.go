package source

import (
	"context"
)

// Error is a Source that emits any errors from a channel and closes when the channel is done.
// Useful for simulating intermittent or streaming error cases.
type Error struct {
	Errs <-chan error
}

func (s *Error) Read(ctx context.Context) (<-chan interface{}, <-chan error) {
	out := make(chan interface{})
	errs := make(chan error)

	go func() {
		defer close(out)
		defer close(errs)
		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-s.Errs:
				if !ok {
					return
				}
				errs <- err
			}
		}
	}()

	return out, errs
}
