package sync_test

import (
	"context"
	"fmt"
	"log"
	stdsync "sync"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/sync"
)

// Example demonstrates basic usage of BatchReader for batching read operations.
func Example_batchReader() {
	// Define a function that performs batched reads from your data source
	readFunc := func(ctx context.Context, keys []string) (map[string]string, error) {
		// In a real application, this would query a database, cache, or API
		result := make(map[string]string)
		for _, key := range keys {
			result[key] = fmt.Sprintf("value-%s", key)
		}
		return result, nil
	}

	// Configure batching behavior
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MaxItems: 10,                    // Process at most 10 items per batch
		MaxTime:  50 * time.Millisecond, // Wait at most 50ms before processing
	})

	// Create the batch reader
	reader := sync.NewBatchReader(config, readFunc)
	defer reader.Close()

	// Make individual calls that are batched behind the scenes
	value, err := reader.Get(context.Background(), "user:123")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(value)
	// Output: value-user:123
}

// Example_batchWriter demonstrates basic usage of BatchWriter for batching write operations.
func Example_batchWriter() {
	// Track writes for the example
	var mu stdsync.Mutex
	written := make(map[string]string)

	// Define a function that performs batched writes to your data source
	writeFunc := func(ctx context.Context, data map[string]string) error {
		// In a real application, this would write to a database, cache, or API
		mu.Lock()
		for k, v := range data {
			written[k] = v
		}
		mu.Unlock()
		return nil
	}

	// Configure batching behavior
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MaxItems: 5,                     // Process at most 5 items per batch
		MaxTime:  20 * time.Millisecond, // Wait at most 20ms before processing
	})

	// Create the batch writer
	writer := sync.NewBatchWriter(config, writeFunc)
	defer writer.Close()

	// Make individual calls that are batched behind the scenes
	err := writer.Set(context.Background(), "user:123", "John Doe")
	if err != nil {
		log.Fatal(err)
	}

	// Wait a moment to ensure batch is processed
	time.Sleep(30 * time.Millisecond)

	mu.Lock()
	fmt.Println(written["user:123"])
	mu.Unlock()
	// Output: John Doe
}

// Example_concurrentOperations demonstrates how multiple concurrent operations
// are automatically batched together for efficiency.
func Example_concurrentOperations() {
	// Track batch calls
	var callCount int
	var mu stdsync.Mutex

	readFunc := func(ctx context.Context, keys []string) (map[string]string, error) {
		mu.Lock()
		callCount++
		mu.Unlock()

		result := make(map[string]string)
		for _, key := range keys {
			result[key] = "data"
		}
		return result, nil
	}

	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 5,                     // Wait for at least 5 items
		MaxTime:  10 * time.Millisecond, // But process after 10ms regardless
	})

	reader := sync.NewBatchReader(config, readFunc)
	defer reader.Close()

	// Launch 10 concurrent reads
	var wg stdsync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", id)
			_, _ = reader.Get(context.Background(), key)
		}(i)
	}

	wg.Wait()

	mu.Lock()
	fmt.Printf("Batch calls made: %d\n", callCount)
	mu.Unlock()
	// Output: Batch calls made: 2
}

// Example_errorHandling demonstrates how errors are handled in batch operations.
func Example_errorHandling() {
	// Simulate a data source that sometimes fails
	attemptCount := 0
	readFunc := func(ctx context.Context, keys []string) (map[string]string, error) {
		attemptCount++
		if attemptCount == 1 {
			return nil, fmt.Errorf("temporary failure")
		}
		return map[string]string{"key": "value"}, nil
	}

	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 1,
	})

	reader := sync.NewBatchReader(config, readFunc)
	defer reader.Close()

	// First call will fail
	_, err := reader.Get(context.Background(), "key")
	if err != nil {
		fmt.Printf("First attempt failed: %v\n", err)
	}

	// Second call will succeed
	value, err := reader.Get(context.Background(), "key")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Second attempt succeeded: %s\n", value)
	// Output:
	// First attempt failed: temporary failure
	// Second attempt succeeded: value
}

// Example_contextCancellation demonstrates how context cancellation is handled.
func Example_contextCancellation() {
	readFunc := func(ctx context.Context, keys []string) (map[string]string, error) {
		// Simulate a slow operation
		select {
		case <-time.After(100 * time.Millisecond):
			return map[string]string{"key": "value"}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 2,                      // Wait for 2 items
		MaxTime:  200 * time.Millisecond, // Long timeout
	})

	reader := sync.NewBatchReader(config, readFunc)
	defer reader.Close()

	// Create a context that cancels quickly
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := reader.Get(ctx, "key")
	if err != nil {
		fmt.Printf("Operation cancelled: %v\n", err)
	}
	// Output: Operation cancelled: context deadline exceeded
}
