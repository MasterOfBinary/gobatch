package processor

import (
	"context"
	"sync"

	"github.com/MasterOfBinary/gobatch/batch"
)

// ResultCollector is a processor that collects processed items.
// It can be used as the final processor in a chain to collect results.
// The processor implements thread-safe collection of items that pass through
// the processor chain, allowing access to the collected items after processing
// is complete.
//
// Example usage:
//
//	collector := &processor.ResultCollector{}
//	b := batch.New(config)
//	batch.RunBatchAndWait(ctx, b, source, processor1, processor2, collector)
//	
//	// Access results
//	results := collector.Results()
//	for _, item := range results {
//	  fmt.Println(item.Data)
//	}
//
// By default, items with errors are not collected. This behavior can be
// changed by setting CollectErrors to true.
type ResultCollector struct {
	// Filter determines which items to collect.
	// If nil, all items without errors are collected.
	// For an item to be collected, this function must return true.
	Filter func(item *batch.Item) bool

	// MaxItems limits the number of items collected (0 for unlimited).
	// Once the number of collected items reaches MaxItems, no more items
	// will be collected.
	MaxItems int

	// CollectErrors determines whether to collect items with errors.
	// When false (default), items with non-nil Error will be skipped.
	// When true, items with errors will be collected if they pass the Filter.
	CollectErrors bool

	mu      sync.Mutex
	results []*batch.Item
}

// Process implements the Processor interface by collecting items
// based on the filter criteria. All items pass through unchanged,
// allowing subsequent processors to operate on them.
func (c *ResultCollector) Process(_ context.Context, items []*batch.Item) ([]*batch.Item, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, item := range items {
		// Skip items with errors unless CollectErrors is true
		if item.Error != nil && !c.CollectErrors {
			continue
		}

		// Apply custom filter if present
		if c.Filter != nil && !c.Filter(item) {
			continue
		}

		// Check if we've reached the maximum number of items
		if c.MaxItems > 0 && len(c.results) >= c.MaxItems {
			break
		}

		// Create a deep copy of the item to avoid issues if later processors modify it
		itemCopy := &batch.Item{
			ID:    item.ID,
			Data:  item.Data,
			Error: item.Error,
		}
		c.results = append(c.results, itemCopy)
	}

	return items, nil
}

// Results returns a deep copy of the collected items.
// This method is thread-safe and can be called while processing is ongoing.
func (c *ResultCollector) Results() []*batch.Item {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Create a deep copy to prevent external modifications from affecting internal state
	result := make([]*batch.Item, len(c.results))
	for i, item := range c.results {
		// Make a deep copy of each item
		result[i] = &batch.Item{
			ID:    item.ID,
			Data:  item.Data,
			Error: item.Error,
		}
	}
	return result
}

// Reset clears all collected results.
// This method is thread-safe and can be used to reuse the collector.
func (c *ResultCollector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.results = nil
}

// Count returns the number of items collected so far.
// This method is thread-safe and can be called while processing is ongoing.
func (c *ResultCollector) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.results)
}

// ExtractData extracts typed data from collected results.
// Only items where the Data field can be type-asserted to T are included.
// Items with errors are included only if they were collected.
//
// Example:
//
//	collector := &processor.ResultCollector{}
//	batch.RunBatchAndWait(ctx, b, src, processor1, collector)
//	
//	// Extract all string values
//	strings := processor.ExtractData[string](collector)
func ExtractData[T any](collector *ResultCollector) []T {
	items := collector.Results()
	result := make([]T, 0, len(items))

	for _, item := range items {
		if item.Error != nil {
			continue
		}

		if data, ok := item.Data.(T); ok {
			result = append(result, data)
		}
	}

	return result
}