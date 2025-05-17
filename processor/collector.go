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
// Thread-Safety Guarantees:
//
// - All methods are thread-safe and can be called concurrently from multiple goroutines.
// - The Process method uses an exclusive write lock when adding items to the collection.
// - The Results method uses a read lock when reset=false, allowing concurrent reads.
// - The Results method uses an exclusive write lock when reset=true.
// - The Count method uses a read lock, allowing concurrent counting with other read operations.
// - The Reset method uses an exclusive write lock to clear the collection.
//
// Appropriate Usage Patterns:
//
// - In most cases, use Results(false) to read results without clearing.
// - Use Results(true) to retrieve items and clear the collection in one atomic operation.
// - When multiple goroutines need to read results concurrently, all can call Results(false)
//   simultaneously without blocking each other.
// - When writing a custom function to extract typed data, use Results(false) to get the items,
//   then manually filter them based on type and error state.
//
// Data Handling Considerations:
//
// Important: ResultCollector creates copies of batch.Item structs but does not
// perform deep copying of the Data field contents. If Data contains mutable
// reference types (maps, slices, pointers to structs), modifications to those
// objects after collection may be reflected in the collected results. For complete
// isolation, ensure Data contains only immutable types or perform manual deep
// copying when accessing results.
//
// Performance Considerations:
//
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
//
// This method acquires an exclusive write lock while collecting items to ensure
// thread safety. Since it's part of the batch processing pipeline, this lock is
// typically not contended unless results are being accessed concurrently.
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
// Thread safety:
// - When reset=false, a read lock is acquired, allowing multiple concurrent readers
// - When reset=true, an exclusive write lock is acquired to clear the collection
// - Multiple goroutines can call Results(false) concurrently without blocking each other
// - Calls to Results(true) will block until no other readers or writers are active
//
// Note: While this method creates new batch.Item structs, it does not deep-copy
// the Data field contents. If Data contains mutable objects, they will still
// reference the same underlying data.
func (c *ResultCollector) Results(reset bool) []*batch.Item {
	// Use appropriate lock based on whether we're resetting
	if reset {
		c.mu.Lock()
		defer c.mu.Unlock()
	} else {
		c.mu.RLock()
		defer c.mu.RUnlock()
	}

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

	// Reset the collection if requested (we already have exclusive lock if reset is true)
	if reset {
		c.results = nil
	}

	return result
}

// Reset clears all collected results.
// This method is thread-safe and can be used to reuse the collector.
//
// This method acquires an exclusive write lock to safely clear the collection.
// If you need to get the results and clear the collection in a single operation,
// prefer using Results(true) for better efficiency.
func (c *ResultCollector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.results = nil
}

// Count returns the number of items collected so far.
// This method is thread-safe and can be called while processing is ongoing.
//
// This method acquires a read lock, allowing multiple concurrent Count calls
// and concurrent calls to Results(false) without blocking each other.
func (c *ResultCollector) Count() int {
	c.mu.RLock() // Use read lock since we're not modifying data
	defer c.mu.RUnlock()

	return len(c.results)
}

