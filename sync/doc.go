// Package sync provides synchronous, blocking APIs for batch operations built on top
// of the gobatch batch processing engine. It offers type-safe, generic interfaces
// for batching read and write operations while making the calls appear synchronous
// to the caller.
//
// The main types are BatchReader and BatchWriter, which provide Get and Set methods
// that block until the operation completes. Behind the scenes, these operations are
// batched together for efficiency.
//
// Basic usage for reading:
//
//	// Define a function that performs batched reads
//	readFunc := func(ctx context.Context, keys []string) (map[string]string, error) {
//		// Perform batched database read, API call, etc.
//		return db.BatchGet(ctx, keys)
//	}
//
//	// Create a batch reader
//	config := batch.NewConstantConfig(&batch.ConfigValues{
//		MaxItems: 10,
//		MaxTime:  50 * time.Millisecond,
//	})
//	reader := sync.NewBatchReader(config, readFunc)
//
//	// Make synchronous calls that are batched behind the scenes
//	value, err := reader.Get(ctx, "key1")
//
// Basic usage for writing:
//
//	// Define a function that performs batched writes
//	writeFunc := func(ctx context.Context, data map[string]string) error {
//		// Perform batched database write, API call, etc.
//		return db.BatchSet(ctx, data)
//	}
//
//	// Create a batch writer
//	writer := sync.NewBatchWriter(config, writeFunc)
//
//	// Make synchronous calls that are batched behind the scenes
//	err := writer.Set(ctx, "key1", "value1")
//
// The sync package handles:
//   - Per-request context cancellation
//   - Request queuing and batching
//   - Error propagation (both global batch errors and per-item errors)
//   - Graceful shutdown
//
// This package is ideal for scenarios where you want the efficiency of batching
// but need a synchronous API, such as:
//   - Caching layers
//   - Database access patterns
//   - External API calls that support bulk operations
package sync
