package source

import (
	"context"
)

// Channel is a Source that reads from an input channel until it's closed.
// It simplifies using an existing channel as a data source for batch processing.
type Channel struct {
	// Input is the channel from which this source will read data.
	// The Channel source will not close this channel.
	Input <-chan interface{}
	// BufferSize controls the size of the output buffer (default: 100)
	BufferSize int
}

// Read implements the Source interface by forwarding items from the Input channel
// to the output channel until Input is closed or context is canceled.
//
// The returned channels are always created (never nil) and always closed properly
// when the source is done providing data or context is cancelled.
func (s *Channel) Read(ctx context.Context) (<-chan interface{}, <-chan error) {
	bufSize := 100
	if s.BufferSize > 0 {
		bufSize = s.BufferSize
	}

	out := make(chan interface{}, bufSize)
	errs := make(chan error)

	go func() {
		defer close(out)
		defer close(errs)

		if s.Input == nil {
			// Handle nil input gracefully
			return
		}

		for {
			select {
			case <-ctx.Done():
				return
			case item, ok := <-s.Input:
				if !ok {
					return
				}
				select {
				case <-ctx.Done():
					return
				case out <- item:
					// Item sent successfully
				}
			}
		}
	}()

	return out, errs
}
