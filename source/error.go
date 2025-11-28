package source

import (
	"context"
)

// defaultErrorBuffer is used when BufferSize is zero to set the capacity of the
// error channel created by Error.Read.
const defaultErrorBuffer = 10

// Error is a Source that only emits errors from a channel and provides no data.
// It is useful for testing error handling in batch processing pipelines and
// for representing error-only streams.
//
// Deprecated: Use NewError() to create an Error source with validation.
type Error struct {
	// Errs is the channel from which this source will read errors.
	// The Error source will not close this channel.
	Errs <-chan error
	// BufferSize controls the size of the error buffer (default: 10)
	BufferSize int
}

// Read implements the Source interface by forwarding errors from the Errs channel
// to the error channel until Errs is closed or context is canceled.
//
// The returned channels are always created (never nil) and always closed properly
// when the source is done providing errors or context is canceled.
// The output channel is always empty, as this source produces only errors.
func (s *Error) Read(ctx context.Context) (<-chan interface{}, <-chan error) {
	out := make(chan interface{})

	bufSize := defaultErrorBuffer
	if s.BufferSize > 0 {
		bufSize = s.BufferSize
	}
	errs := make(chan error, bufSize)

	go func() {
		defer close(out)
		defer close(errs)

		if s.Errs == nil {
			// Handle nil error channel gracefully
			return
		}

		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-s.Errs:
				if !ok {
					return
				}
				// Only forward non-nil errors
				if err != nil {
					select {
					case <-ctx.Done():
						return
					case errs <- err:
						// Error sent successfully
					}
				}
			}
		}
	}()

	return out, errs
}
