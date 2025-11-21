package processor

import (
	"context"

	"github.com/MasterOfBinary/gobatch/batch"
)

// TransformFunc is a function that transforms an item's Data field.
// It takes the current Data value and returns the new Data value.
type TransformFunc func(data interface{}) (interface{}, error)

// Transform is a processor that applies a transformation function to each item's Data field.
// It can be used to convert, modify, or restructure data during batch processing.
type Transform struct {
	// Func is the transformation function to apply to each item's Data field.
	// If nil, items pass through unchanged.
	Func TransformFunc

	// StopOnError determines whether to stop processing items after a transformation error.
	// If true, the processor will return an error and stop after the first transformation failure.
	// If false, items with transformation errors will have their Error field set but processing continues.
	// Default is false (continue processing).
	StopOnError bool
}

// Process implements the Processor interface by applying the transformation function
// to each item's Data field.
func (p *Transform) Process(_ context.Context, items []*batch.Item) ([]*batch.Item, error) {
	if len(items) == 0 || p.Func == nil {
		return items, nil
	}

	for _, item := range items {
		if item.Error != nil {
			// Skip items that already have errors
			continue
		}

		newData, err := p.Func(item.Data)
		if err != nil {
			item.Error = err

			if p.StopOnError {
				return items, err
			}
		} else {
			item.Data = newData
		}
	}

	return items, nil
}
