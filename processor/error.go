package processor

import (
	"context"
	"errors"

	"github.com/MasterOfBinary/gobatch/batch"
)

// Error is a Processor that marks all incoming items with the given error.
// It's useful for testing error handling in batch processing pipelines and
// for simulating scenarios where items fail processing.
type Error struct {
	// Err is the error to apply to each item.
	// If nil, a default "processor error" will be used.
	Err error

	// FailFraction controls what fraction of items should have errors applied.
	// Value range is 0.0 to 1.0, where:
	// - 0.0 means no items will have errors (processor becomes a pass-through)
	// - 1.0 means all items will have errors (default)
	// - 0.5 means approximately half the items will have errors
	FailFraction float64
}

// Process implements the Processor interface by marking items with errors
// according to the configured FailFraction.
func (p *Error) Process(_ context.Context, items []*batch.Item) ([]*batch.Item, error) {
	if len(items) == 0 {
		return items, nil
	}

	err := p.Err
	if err == nil {
		err = errors.New("processor error")
	}

	failFraction := p.FailFraction

	// Fast-path: no failures requested.
	if failFraction <= 0 {
		return items, nil
	}

	// Fast-path: all items fail.
	if failFraction >= 1.0 {
		for _, item := range items {
			item.Error = err
		}
		return items, nil
	}

	// Deterministically approximate the requested failure fraction using an
	// accumulator. This avoids the previous logic that produced 100 % failures
	// for any FailFraction > 0.5.
	//
	// The algorithm adds the fraction to an accumulator on every item. When the
	// accumulator crosses 1.0 we mark the item as failed and decrement the
	// accumulator. This evenly distributes failures across the slice while
	// honouring the requested ratio.
	acc := 0.0
	for _, item := range items {
		acc += failFraction
		if acc >= 1.0 {
			item.Error = err
			acc -= 1.0
		}
	}

	return items, nil
}
