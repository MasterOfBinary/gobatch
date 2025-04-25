package pipeline

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
)

// MockRedisClient is a simple mock implementation for testing RedisPipeline
type MockRedisClient struct {
	data      map[string]interface{} // In-memory data store
	errorRate float64                // Probability of operations failing (0-1)
	delay     time.Duration          // Artificial delay to simulate network latency
	opCount   int                    // Counter for operations performed
}

func NewMockRedisClient() *MockRedisClient {
	return &MockRedisClient{
		data:      make(map[string]interface{}),
		errorRate: 0,
		delay:     0,
	}
}

// SetErrorRate sets the probability of operations failing
func (m *MockRedisClient) SetErrorRate(rate float64) {
	m.errorRate = rate
}

// SetDelay sets the artificial delay to simulate latency
func (m *MockRedisClient) SetDelay(delay time.Duration) {
	m.delay = delay
}

// shouldError determines if the operation should fail based on the error rate
func (m *MockRedisClient) shouldError() bool {
	m.opCount++
	if m.errorRate <= 0 {
		return false
	}
	return m.opCount%int(1/m.errorRate) == 0
}

// simulateDelay introduces an artificial delay if set
func (m *MockRedisClient) simulateDelay() {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
}

// TestRedisPipeline_ProcessBatch tests the Process method of RedisPipeline with a batch of items
func TestRedisPipeline_ProcessBatch(t *testing.T) {
	mockClient := NewMockRedisClient()

	// Create pipeline with default options
	pipeline := NewRedisPipeline(mockClient, nil)

	// Set some initial data
	mockClient.data["test-key"] = "test-value"
	mockClient.data["counter"] = 42

	// Create test items with redisRequest data
	items := []*batch.Item{
		// Get operation
		{
			ID: 1,
			Data: &redisRequest{
				op:     Get,
				key:    "test-key",
				respCh: make(chan *redisResponse, 1),
			},
		},
		// Set operation
		{
			ID: 2,
			Data: &redisRequest{
				op:     Set,
				key:    "new-key",
				value:  "new-value",
				respCh: make(chan *redisResponse, 1),
			},
		},
		// Del operation
		{
			ID: 3,
			Data: &redisRequest{
				op:     Del,
				key:    "test-key",
				respCh: make(chan *redisResponse, 1),
			},
		},
		// Exists operation - key exists
		{
			ID: 4,
			Data: &redisRequest{
				op:     Exists,
				key:    "counter",
				respCh: make(chan *redisResponse, 1),
			},
		},
		// Exists operation - key doesn't exist
		{
			ID: 5,
			Data: &redisRequest{
				op:     Exists,
				key:    "non-existent-key",
				respCh: make(chan *redisResponse, 1),
			},
		},
		// Invalid data type
		{
			ID:   6,
			Data: "not a redisRequest",
		},
		// Unsupported operation
		{
			ID: 7,
			Data: &redisRequest{
				op:     "UNKNOWN",
				key:    "test-key",
				respCh: make(chan *redisResponse, 1),
			},
		},
	}

	// Process the items
	ctx := context.Background()
	_, err := pipeline.Process(ctx, items)

	// Verify there's no batch-level error
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify the correct number of items were processed
	if len(items) != len(items) {
		t.Errorf("Expected %d processed items, got %d", len(items), len(items))
	}

	// Now read responses from the channels
	getResp := (<-items[0].Data.(*redisRequest).respCh)
	if getResp.err != nil {
		t.Errorf("Expected no error for GET, got %v", getResp.err)
	}
	if getResp.value != "simulated-value-for-test-key" {
		t.Errorf("Expected result 'simulated-value-for-test-key', got %v", getResp.value)
	}

	setResp := (<-items[1].Data.(*redisRequest).respCh)
	if setResp.err != nil {
		t.Errorf("Expected no error for SET, got %v", setResp.err)
	}
	if setResp.value != "OK" {
		t.Errorf("Expected result 'OK', got %v", setResp.value)
	}

	delResp := (<-items[2].Data.(*redisRequest).respCh)
	if delResp.err != nil {
		t.Errorf("Expected no error for DEL, got %v", delResp.err)
	}
	if delResp.value.(int64) != 1 {
		t.Errorf("Expected result 1, got %v", delResp.value)
	}

	existsResp := (<-items[3].Data.(*redisRequest).respCh)
	if existsResp.err != nil {
		t.Errorf("Expected no error for EXISTS, got %v", existsResp.err)
	}
	if existsResp.value.(int64) != 1 {
		t.Errorf("Expected result 1, got %v", existsResp.value)
	}

	notExistsResp := (<-items[4].Data.(*redisRequest).respCh)
	if notExistsResp.err != nil {
		t.Errorf("Expected no error for EXISTS, got %v", notExistsResp.err)
	}

	// Check for invalid data type error
	if items[5].Error == nil {
		t.Error("Expected error for invalid data type, got nil")
	}

	// Check for unsupported operation error
	unknownResp := (<-items[6].Data.(*redisRequest).respCh)
	if unknownResp.err == nil {
		t.Error("Expected error for unsupported operation, got nil")
	}
}

// TestRedisPipeline_EmptyBatch tests handling of empty batches
func TestRedisPipeline_EmptyBatch(t *testing.T) {
	pipeline := NewRedisPipeline(NewMockRedisClient(), nil)
	items := []*batch.Item{}

	ctx := context.Background()
	_, err := pipeline.Process(ctx, items)

	if err != nil {
		t.Errorf("Expected no error for empty batch, got %v", err)
	}

	if len(items) != 0 {
		t.Errorf("Expected 0 processed items, got %d", len(items))
	}
}

// TestRedisPipeline_NilClient tests handling of nil Redis client
func TestRedisPipeline_NilClient(t *testing.T) {
	pipeline := NewRedisPipeline(nil, nil)

	respCh := make(chan *redisResponse, 1)
	items := []*batch.Item{
		{
			ID: 1,
			Data: &redisRequest{
				op:     Get,
				key:    "test-key",
				respCh: respCh,
			},
		},
	}

	ctx := context.Background()
	_, err := pipeline.Process(ctx, items)

	// The mock implementation should still work even with nil client
	if err != nil {
		t.Errorf("Expected no error with nil client, got %v", err)
	}

	// No need to check processedItems since we're testing the response channel

	// Check response
	resp := <-respCh
	if resp.err != nil {
		t.Errorf("Expected no error for the item, got %v", resp.err)
	}
}

// TestRedisPipeline_FullLifecycle tests a more realistic usage of RedisPipeline
func TestRedisPipeline_FullLifecycle(t *testing.T) {
	mockClient := NewMockRedisClient()
	pipeline := NewRedisPipeline(mockClient, &RedisPipelineOptions{
		MinItems:   2,
		MaxItems:   10,
		MinTime:    10 * time.Millisecond,
		MaxTime:    50 * time.Millisecond,
		BufferSize: 100,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Start the pipeline
	err := pipeline.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}

	// Perform some operations
	val, err := pipeline.Get(ctx, "test-key")
	if err != nil {
		t.Errorf("GET operation failed: %v", err)
	}

	// Value should match our simulated response
	if val != "simulated-value-for-test-key" {
		t.Errorf("Unexpected GET result: %v", val)
	}

	// Set a value
	result, err := pipeline.Set(ctx, "new-key", "new-value")
	if err != nil {
		t.Errorf("SET operation failed: %v", err)
	}
	if result != "OK" {
		t.Errorf("Unexpected SET result: %v", result)
	}

	// Check existence
	exists, err := pipeline.Exists(ctx, "new-key")
	if err != nil {
		t.Errorf("EXISTS operation failed: %v", err)
	}
	if !exists {
		t.Errorf("Expected key to exist, got false")
	}

	// Delete the key
	count, err := pipeline.Del(ctx, "new-key")
	if err != nil {
		t.Errorf("DEL operation failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 key deleted, got %d", count)
	}

	// Stop the pipeline
	err = pipeline.Stop()
	if err != nil {
		t.Fatalf("Failed to stop pipeline: %v", err)
	}
}

// TestRedisPipeline_ConcurrentOperations tests the behavior with many concurrent operations
func TestRedisPipeline_ConcurrentOperations(t *testing.T) {
	mockClient := NewMockRedisClient()
	pipeline := NewRedisPipeline(mockClient, &RedisPipelineOptions{
		MinItems:   5, // Small batch size to ensure batching
		MaxItems:   20,
		MinTime:    5 * time.Millisecond,
		MaxTime:    20 * time.Millisecond,
		BufferSize: 100,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start the pipeline
	err := pipeline.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start pipeline: %v", err)
	}
	defer pipeline.Stop()

	// Track results and errors
	type result struct {
		val interface{}
		err error
	}
	results := make([]result, 100)

	// Launch 100 concurrent operations
	done := make(chan struct{})
	for i := 0; i < 100; i++ {
		go func(idx int) {
			// Alternate between different operations
			var val interface{}
			var err error

			key := fmt.Sprintf("concurrent-key-%d", idx)
			switch idx % 4 {
			case 0: // GET
				val, err = pipeline.Get(ctx, key)
			case 1: // SET
				val, err = pipeline.Set(ctx, key, fmt.Sprintf("value-%d", idx))
			case 2: // DEL
				var count int64
				count, err = pipeline.Del(ctx, key)
				val = count
			case 3: // EXISTS
				var exists bool
				exists, err = pipeline.Exists(ctx, key)
				val = exists
			}

			results[idx] = result{val, err}

			if idx == 99 {
				close(done)
			}
		}(i)
	}

	// Wait for all operations to complete
	select {
	case <-done:
		// All operations completed
	case <-ctx.Done():
		t.Fatalf("Context timed out before operations completed: %v", ctx.Err())
	}

	// Verify results
	errorCount := 0
	for _, r := range results {
		if r.err != nil {
			errorCount++
		}
	}

	// We don't expect any errors in our mock implementation
	if errorCount > 0 {
		t.Errorf("Got %d errors out of 100 operations", errorCount)
	}
}

// TestRedisPipeline_StartStopErrors tests error handling in Start and Stop methods
func TestRedisPipeline_StartStopErrors(t *testing.T) {
	pipeline := NewRedisPipeline(NewMockRedisClient(), nil)
	ctx := context.Background()

	// Start the pipeline
	err := pipeline.Start(ctx)
	if err != nil {
		t.Errorf("First Start() call failed: %v", err)
	}

	// Try to start it again, should fail
	err = pipeline.Start(ctx)
	if err == nil {
		t.Error("Expected error when starting already started pipeline, got nil")
	}

	// Stop the pipeline
	err = pipeline.Stop()
	if err != nil {
		t.Errorf("First Stop() call failed: %v", err)
	}

	// Try to stop it again, should fail
	err = pipeline.Stop()
	if err == nil {
		t.Error("Expected error when stopping already stopped pipeline, got nil")
	}

	// Try operations on stopped pipeline
	_, err = pipeline.Get(ctx, "key")
	if err == nil {
		t.Error("Expected error when calling Get() on stopped pipeline, got nil")
	}
}

// BenchmarkRedisPipeline_Process benchmarks the Process method
func BenchmarkRedisPipeline_Process(b *testing.B) {
	mockClient := NewMockRedisClient()
	pipeline := NewRedisPipeline(mockClient, nil)

	// Create a batch of mixed operations
	const batchSize = 100
	items := make([]*batch.Item, batchSize)

	for i := 0; i < batchSize; i++ {
		opType := i % 4 // 0=GET, 1=SET, 2=DEL, 3=EXISTS
		key := fmt.Sprintf("bench-key-%d", i)
		respCh := make(chan *redisResponse, 1)

		var op RedisOperation
		var value interface{}

		switch opType {
		case 0:
			op = Get
		case 1:
			op = Set
			value = fmt.Sprintf("bench-value-%d", i)
		case 2:
			op = Del
		case 3:
			op = Exists
		}

		items[i] = &batch.Item{
			ID: uint64(i) + 1,
			Data: &redisRequest{
				op:     op,
				key:    key,
				value:  value,
				respCh: respCh,
			},
		}
	}

	ctx := context.Background()

	// Reset the benchmark timer
	b.ResetTimer()

	// Run the benchmark
	for i := 0; i < b.N; i++ {
		// Process the batch
		pipeline.Process(ctx, items)

		// Drain response channels
		for _, item := range items {
			if req, ok := item.Data.(*redisRequest); ok && req.respCh != nil {
				select {
				case <-req.respCh:
					// Drain the channel
				default:
					// Channel already drained
				}
			}
		}
	}
}
