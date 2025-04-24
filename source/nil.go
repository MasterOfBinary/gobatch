package source

import (
	"context"
	"time"
)

// Nil is a Source that does nothing but sleeps for a given duration before closing.
// Useful for testing shutdown and timing.
type Nil struct {
	Duration time.Duration
}

// Read blocks for the given duration and then closes the channels.
func (s *Nil) Read(ctx context.Context) (<-chan interface{}, <-chan error) {
	out := make(chan interface{})
	errs := make(chan error)

	go func() {
		defer close(out)
		defer close(errs)

		select {
		case <-time.After(s.Duration):
		case <-ctx.Done():
		}
	}()

	return out, errs
}
