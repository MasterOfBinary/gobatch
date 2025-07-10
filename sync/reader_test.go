package sync

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
)

func TestBatchReader_SingleGet(t *testing.T) {
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 1,
		MaxItems: 10,
		MaxTime:  10 * time.Millisecond,
	})

	readFunc := func(ctx context.Context, keys []string) (map[string]string, error) {
		result := make(map[string]string)
		for _, key := range keys {
			result[key] = "value-" + key
		}
		return result, nil
	}

	reader := NewBatchReader(config, readFunc)
	defer reader.Close()

	ctx := context.Background()
	value, err := reader.Get(ctx, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != "value-test" {
		t.Errorf("expected value-test, got %s", value)
	}
}

func TestBatchReader_ConcurrentGets(t *testing.T) {
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 5,
		MaxItems: 10,
		MaxTime:  50 * time.Millisecond,
	})

	callCount := 0
	var mu sync.Mutex

	readFunc := func(ctx context.Context, keys []string) (map[string]string, error) {
		mu.Lock()
		callCount++
		mu.Unlock()

		result := make(map[string]string)
		for _, key := range keys {
			result[key] = "value-" + key
		}
		return result, nil
	}

	reader := NewBatchReader(config, readFunc)
	defer reader.Close()

	// Launch concurrent reads
	var wg sync.WaitGroup
	results := make([]string, 10)
	errors := make([]error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := string(rune('a' + idx))
			results[idx], errors[idx] = reader.Get(context.Background(), key)
		}(i)
	}

	wg.Wait()

	// Verify results
	for i := 0; i < 10; i++ {
		if errors[i] != nil {
			t.Errorf("unexpected error for index %d: %v", i, errors[i])
		}
		expected := "value-" + string(rune('a'+i))
		if results[i] != expected {
			t.Errorf("expected %s, got %s", expected, results[i])
		}
	}

	// Should have batched into 2 calls (10 items / 5 min items)
	mu.Lock()
	if callCount != 2 {
		t.Errorf("expected 2 batch calls, got %d", callCount)
	}
	mu.Unlock()
}

func TestBatchReader_ContextCancellation(t *testing.T) {
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 5,
		MaxTime:  100 * time.Millisecond,
	})

	readFunc := func(ctx context.Context, keys []string) (map[string]string, error) {
		// Simulate slow operation
		time.Sleep(50 * time.Millisecond)
		return map[string]string{"key": "value"}, nil
	}

	reader := NewBatchReader(config, readFunc)
	defer reader.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := reader.Get(ctx, "key")
	if err != context.Canceled {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}

func TestBatchReader_KeyNotFound(t *testing.T) {
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 1,
	})

	readFunc := func(ctx context.Context, keys []string) (map[string]string, error) {
		// Return empty map - key not found
		return map[string]string{}, nil
	}

	reader := NewBatchReader(config, readFunc)
	defer reader.Close()

	_, err := reader.Get(context.Background(), "missing")
	if err == nil || err.Error() != "key not found" {
		t.Errorf("expected 'key not found' error, got %v", err)
	}
}

func TestBatchReader_BatchError(t *testing.T) {
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 1,
	})

	expectedErr := errors.New("database error")
	readFunc := func(ctx context.Context, keys []string) (map[string]string, error) {
		return nil, expectedErr
	}

	reader := NewBatchReader(config, readFunc)
	defer reader.Close()

	_, err := reader.Get(context.Background(), "key")
	if err != expectedErr {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

func TestBatchReader_NilContext(t *testing.T) {
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 1,
	})

	readFunc := func(ctx context.Context, keys []string) (map[string]string, error) {
		return map[string]string{"key": "value"}, nil
	}

	reader := NewBatchReader(config, readFunc)
	defer reader.Close()

	// Should not panic with nil context
	value, err := reader.Get(nil, "key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != "value" {
		t.Errorf("expected value, got %s", value)
	}
}

func TestBatchReader_MultipleClose(t *testing.T) {
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 1,
	})

	readFunc := func(ctx context.Context, keys []string) (map[string]string, error) {
		return nil, nil
	}

	reader := NewBatchReader(config, readFunc)

	// Multiple closes should not panic
	reader.Close()
	reader.Close()
}

// Benchmark tests
func BenchmarkBatchReader_SingleGet(b *testing.B) {
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 1,
		MaxItems: 100,
		MaxTime:  10 * time.Millisecond,
	})

	readFunc := func(ctx context.Context, keys []string) (map[string]string, error) {
		result := make(map[string]string)
		for _, key := range keys {
			result[key] = "value"
		}
		return result, nil
	}

	reader := NewBatchReader(config, readFunc)
	defer reader.Close()

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = reader.Get(ctx, "key")
	}
}

func BenchmarkBatchReader_ConcurrentGets(b *testing.B) {
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 10,
		MaxItems: 100,
		MaxTime:  10 * time.Millisecond,
	})

	readFunc := func(ctx context.Context, keys []string) (map[string]string, error) {
		result := make(map[string]string)
		for _, key := range keys {
			result[key] = "value"
		}
		return result, nil
	}

	reader := NewBatchReader(config, readFunc)
	defer reader.Close()

	ctx := context.Background()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = reader.Get(ctx, "key")
		}
	})
}
