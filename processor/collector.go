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
// Important: ResultCollector creates copies of batch.Item structs but does not
// perform deep copying of the Data field contents. If Data contains mutable
// reference types (maps, slices, pointers to structs), modifications to those
// objects after collection may be reflected in the collected results. For complete
// isolation, ensure Data contains only immutable types or perform manual deep
// copying when accessing results.
//
// Performance Considerations:
// For extremely high-throughput workloads with many concurrent batches, using
// ResultCollector may introduce lock contention since it acquires a lock during
// each batch processing. In such cases, consider:
// - Using multiple collector instances (one per group of processors)
// - Collecting only a subset of items with the Filter option
// - Limiting collected items with the MaxItems option
// - Using a custom processor that uses more specialized concurrency patterns
//
// Example usage:
//
//	collector := &processor.ResultCollector{}
//	b := batch.New(config)
//	batch.RunBatchAndWait(ctx, b, source, processor1, processor2, collector)
//	
//	// Access results without resetting
//	results := collector.Results(false)
//	for _, item := range results {
//	  fmt.Println(item.Data)
//	}
//	
//	// Or get results and reset in one operation
//	finalResults := collector.Results(true)
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

	mu      sync.RWMutex // Using RWMutex allows concurrent reads while ensuring exclusive writes
	results []*batch.Item
}

// Process implements the Processor interface by collecting items
// based on the filter criteria. All items pass through unchanged,
// allowing subsequent processors to operate on them.
func (c *ResultCollector) Process(_ context.Context, items []*batch.Item) ([]*batch.Item, error) {
	c.mu.Lock() // Exclusive lock for writing
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

		// Create a copy of the item struct (note: Data field is not deep-copied)
		itemCopy := &batch.Item{
			ID:    item.ID,
			Data:  item.Data, // Only copies the reference for reference types
			Error: item.Error,
		}
		c.results = append(c.results, itemCopy)
	}

	return items, nil
}

// Results returns a copy of the collected items and optionally resets the collection.
// This method is thread-safe and can be called while processing is ongoing.
//
// The reset parameter determines whether to clear the collection after retrieving results:
// - If reset is true, all collected items are cleared after being returned (atomic get-and-reset)
// - If reset is false, items remain in the collection for future retrieval
//
// Note: While this method creates new batch.Item structs, it does not deep-copy
// the Data field contents. If Data contains mutable objects, they will still
// reference the same underlying data.
func (c *ResultCollector) Results(reset bool) []*batch.Item {
	c.mu.Lock() // Need exclusive lock if resetting
	defer c.mu.Unlock()

	// Create a copy of each item to prevent modifications to the Item structs
	result := make([]*batch.Item, len(c.results))
	for i, item := range c.results {
		// Create a new Item (note: Data field is not deep-copied)
		result[i] = &batch.Item{
			ID:    item.ID,
			Data:  item.Data,
			Error: item.Error,
		}
	}

	// Reset the collection if requested
	if reset {
		c.results = nil
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
	c.mu.RLock() // Use read lock since we're not modifying data
	defer c.mu.RUnlock()

	return len(c.results)
}

// ExtractData extracts typed data from collected results.
// Only items where the Data field can be type-asserted to T are included.
// Items with errors are included only if they were collected.
//
// The reset parameter determines whether to clear the collection after extracting data.
//
// Example:
//
//	collector := &processor.ResultCollector{}
//	batch.RunBatchAndWait(ctx, b, src, processor1, collector)
//	
//	// Extract all string values (keep items in collector)
//	strings := processor.ExtractData[string](collector, false)
//	
//	// Extract and clear collection
//	finalStrings := processor.ExtractData[string](collector, true)
func ExtractData[T any](collector *ResultCollector, reset bool) []T {
	items := collector.Results(reset)
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