package processor

import (
	"context"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
)

// Nil is a Processor that sleeps for the configured duration and does nothing else.
// It's useful for testing timing behavior and simulating time-consuming operations
// without actually modifying items.
type Nil struct {
	// Duration specifies how long the processor should sleep before returning.
	// If zero or negative, the processor will return immediately.
	Duration time.Duration

	// MarkCancelled controls whether items should be marked with ctx.Err()
	// when the context is cancelled during processing.
	// If false, items are returned unchanged on cancellation.
	MarkCancelled bool
}

// Process implements the Processor interface by waiting for the specified duration
// and returning the items unchanged, unless cancelled.
func (p *Nil) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	if len(items) == 0 || p.Duration <= 0 {
		return items, nil
	}

	timer := time.NewTimer(p.Duration)
	defer timer.Stop()

	select {
	case <-timer.C:
		// Duration complete, return items unchanged
	case <-ctx.Done():
		// Context cancelled
		if p.MarkCancelled {
			for _, item := range items {
				item.Error = ctx.Err()
			}
		}
	}

	return items, nil
}
