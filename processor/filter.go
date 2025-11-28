package processor

import (
	"context"

	"github.com/MasterOfBinary/gobatch/batch"
)

// FilterFunc is a function that decides whether an item should be included in the output.
// Return true to keep the item, false to filter it out.
type FilterFunc func(item *batch.Item) bool

// Filter is a processor that filters items based on a predicate function.
// It can be used to remove items from the pipeline that don't meet certain criteria.
//
// Deprecated: Use NewFilter() to create a Filter processor with validation.
type Filter struct {
	// Predicate is a function that returns true for items that should be kept
	// and false for items that should be filtered out.
	// If nil, no filtering occurs (all items pass through).
	Predicate FilterFunc

	// InvertMatch inverts the predicate logic: if true, items matching the predicate
	// will be removed instead of kept.
	// Default is false (keep matching items).
	InvertMatch bool
}

// Process implements the Processor interface by filtering items according to the predicate.
// Items that don't pass the filter are simply not included in the returned slice.
// This does not set any errors on items, it just excludes them from further processing.
func (p *Filter) Process(_ context.Context, items []*batch.Item) ([]*batch.Item, error) {
	if len(items) == 0 || p.Predicate == nil {
		return items, nil
	}

	// Pre-allocate with capacity of original slice
	result := make([]*batch.Item, 0, len(items))

	for _, item := range items {
		shouldKeep := p.Predicate(item)

		// Invert logic if needed
		if p.InvertMatch {
			shouldKeep = !shouldKeep
		}

		if shouldKeep {
			result = append(result, item)
		}
	}

	return result, nil
}
