// Package processor contains several implementations of the batch.Processor
// interface for common processing scenarios, including:
//
// - Error: For simulating errors with configurable failure rates
// - Filter: For filtering items based on custom predicates
// - Nil: For testing timing behavior without modifying items
// - Transform: For transforming item data values
// - Collect: For capturing processed item data in a slice
//
// Each processor implementation follows a consistent error handling pattern and
// respects context cancellation.
package processor
