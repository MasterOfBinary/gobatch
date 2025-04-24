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
	if failFraction <= 0 {
		// If fraction <= 0, no items fail, just pass-through
		return items, nil
	}

	if failFraction >= 1.0 {
		// Apply error to all items
		for _, item := range items {
			item.Error = err
		}
	} else {
		// Apply error to a fraction of items
		failEvery := int(1.0 / failFraction)
		if failEvery < 1 {
			failEvery = 1
		}

		for i, item := range items {
			if i%failEvery == 0 {
				item.Error = err
			}
		}
	}

	return items, nil
}
