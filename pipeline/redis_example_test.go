package pipeline

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// This example demonstrates how to use the RedisPipeline for batched Redis operations
// with a simple synchronous API.
func Example_redisPipeline() {
	// In a real application, you would use a real Redis client:
	// import "github.com/go-redis/redis/v8"
	// redisClient := redis.NewClient(&redis.Options{
	//     Addr: "localhost:6379",
	// })

	// For this example, we'll use nil since we have a mock implementation
	var redisClient interface{} = nil

	// Create a new RedisPipeline with custom options
	pipeline := NewRedisPipeline(redisClient, &RedisPipelineOptions{
		MinItems:   5,
		MaxItems:   20,
		MinTime:    10 * time.Millisecond,
		MaxTime:    50 * time.Millisecond,
		BufferSize: 100,
	})

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start the pipeline
	if err := pipeline.Start(ctx); err != nil {
		fmt.Printf("Failed to start pipeline: %v\n", err)
		return
	}
	// Check error on stop
	defer func() {
		if err := pipeline.Stop(); err != nil {
			fmt.Printf("Failed to stop pipeline: %v\n", err)
		}
	}()

	// Demonstrate a simple GET operation
	val, err := pipeline.Get(ctx, "user:123")
	if err != nil {
		fmt.Printf("GET error: %v\n", err)
	} else {
		fmt.Printf("GET result: %v\n", val)
	}

	// Demonstrate a SET operation
	result, err := pipeline.Set(ctx, "user:123", "Jane Doe")
	if err != nil {
		fmt.Printf("SET error: %v\n", err)
	} else {
		fmt.Printf("SET result: %v\n", result)
	}

	// Demonstrate multiple concurrent operations
	var wg sync.WaitGroup
	// Use a map to track results by index for ordered output
	concurrentResults := make(map[int]string)
	var resultsMutex sync.Mutex

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("key:%d", idx)
			value := fmt.Sprintf("value:%d", idx)
			var opResult string

			// Set a value
			_, err := pipeline.Set(ctx, key, value)
			if err != nil {
				opResult = fmt.Sprintf("Concurrent SET error for %s: %v", key, err)
				resultsMutex.Lock()
				concurrentResults[idx] = opResult
				resultsMutex.Unlock()
				return
			}

			// Get the value back
			val, err := pipeline.Get(ctx, key)
			if err != nil {
				opResult = fmt.Sprintf("Concurrent GET error for %s: %v", key, err)
			} else {
				// Store the successful result string
				opResult = fmt.Sprintf("Concurrent result %d: %s = %v", idx, key, val)
			}

			resultsMutex.Lock()
			concurrentResults[idx] = opResult
			resultsMutex.Unlock()
		}(i)
	}

	// Wait for all operations to complete
	wg.Wait()

	// Print concurrent results in order
	fmt.Println("Concurrent results:")
	for i := 0; i < 10; i++ {
		resultsMutex.Lock()
		fmt.Println(concurrentResults[i])
		resultsMutex.Unlock()
	}

	// Check if a key exists
	exists, err := pipeline.Exists(ctx, "user:123")
	if err != nil {
		fmt.Printf("EXISTS error: %v\n", err)
	} else {
		fmt.Printf("EXISTS result: %v\n", exists)
	}

	// Delete a key
	deleted, err := pipeline.Del(ctx, "user:123")
	if err != nil {
		fmt.Printf("DEL error: %v\n", err)
	} else {
		fmt.Printf("DEL result: %d\n", deleted)
	}

	// Output:
	// GET result: simulated-value-for-user:123
	// SET result: OK
	// Concurrent results:
	// Concurrent result 0: key:0 = simulated-value-for-key:0
	// Concurrent result 1: key:1 = simulated-value-for-key:1
	// Concurrent result 2: key:2 = simulated-value-for-key:2
	// Concurrent result 3: key:3 = simulated-value-for-key:3
	// Concurrent result 4: key:4 = simulated-value-for-key:4
	// Concurrent result 5: key:5 = simulated-value-for-key:5
	// Concurrent result 6: key:6 = simulated-value-for-key:6
	// Concurrent result 7: key:7 = simulated-value-for-key:7
	// Concurrent result 8: key:8 = simulated-value-for-key:8
	// Concurrent result 9: key:9 = simulated-value-for-key:9
	// EXISTS result: true
	// DEL result: 1
}

// This example demonstrates how the RedisPipeline batches operations for efficiency.
func Example_batchingBehavior() {
	var redisClient interface{} = nil

	// Configure the pipeline to batch operations
	pipeline := NewRedisPipeline(redisClient, &RedisPipelineOptions{
		MinItems:   5,                      // Wait for at least 5 operations
		MaxItems:   20,                     // Process in batches of at most 20
		MinTime:    50 * time.Millisecond,  // Wait at least 50ms
		MaxTime:    200 * time.Millisecond, // But no more than 200ms
		BufferSize: 100,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start the pipeline
	if err := pipeline.Start(ctx); err != nil {
		fmt.Printf("Failed to start pipeline: %v\n", err)
		return
	}
	// Check error on stop
	defer func() {
		if err := pipeline.Stop(); err != nil {
			fmt.Printf("Failed to stop pipeline: %v\n", err)
		}
	}()

	// Submit many operations rapidly - these should be batched
	var wg sync.WaitGroup

	// Use a map to track results by index for ordered output
	resultMap := make(map[int]string)
	var resultMutex sync.Mutex

	// Run 10 operations for more predictable test output
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("batch-key:%d", idx)

			// Each operation returns immediately to the caller even though
			// they might be processed in batches internally
			val, err := pipeline.Get(ctx, key)
			if err != nil {
				resultMutex.Lock()
				resultMap[idx] = fmt.Sprintf("Error: %v", err)
				resultMutex.Unlock()
				return
			}

			resultMutex.Lock()
			// Update Sprintf to remove timing information
			resultMap[idx] = fmt.Sprintf("Operation %d result: %v",
				idx, val)
			resultMutex.Unlock()
		}(i)
	}

	// Wait for all operations to complete
	wg.Wait()

	// Print results in numerical order
	fmt.Println("All operations completed, results:")
	for i := 0; i < 10; i++ {
		fmt.Println(resultMap[i])
	}

	// Output:
	// All operations completed, results:
	// Operation 0 result: simulated-value-for-batch-key:0
	// Operation 1 result: simulated-value-for-batch-key:1
	// Operation 2 result: simulated-value-for-batch-key:2
	// Operation 3 result: simulated-value-for-batch-key:3
	// Operation 4 result: simulated-value-for-batch-key:4
	// Operation 5 result: simulated-value-for-batch-key:5
	// Operation 6 result: simulated-value-for-batch-key:6
	// Operation 7 result: simulated-value-for-batch-key:7
	// Operation 8 result: simulated-value-for-batch-key:8
	// Operation 9 result: simulated-value-for-batch-key:9
}
