package source

import (
	"context"
	"time"
)

// Nil is a Source that does nothing but sleeps for a given duration before closing.
// It is useful for testing shutdown sequences and empty pipeline behavior.
type Nil struct {
	// Duration specifies how long the source will wait before closing its channels.
	// If zero, it will close immediately.
	Duration time.Duration
}

// Read implements the Source interface by waiting for a specified duration
// (or until context cancellation) and then closing the channels.
// It never emits any data or errors.
//
// The returned channels are always created (never nil) and always closed properly
// when the source completes waiting or context is canceled.
func (s *Nil) Read(ctx context.Context) (<-chan interface{}, <-chan error) {
	out := make(chan interface{})
	errs := make(chan error)

	go func() {
		defer close(out)
		defer close(errs)

		// If duration is 0 or negative, close immediately
		if s.Duration <= 0 {
			return
		}

		timer := time.NewTimer(s.Duration)
		defer timer.Stop()

		select {
		case <-timer.C:
			// Duration complete
		case <-ctx.Done():
			// Context canceled
		}
	}()

	return out, errs
}
