package sync

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
)

func TestBatchWriter_SingleSet(t *testing.T) {
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 1,
		MaxItems: 10,
		MaxTime:  10 * time.Millisecond,
	})

	var writtenData map[string]string
	writeFunc := func(ctx context.Context, data map[string]string) error {
		writtenData = data
		return nil
	}

	writer := NewBatchWriter(config, writeFunc)
	defer writer.Close()

	err := writer.Set(context.Background(), "key", "value")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if writtenData["key"] != "value" {
		t.Errorf("expected value, got %s", writtenData["key"])
	}
}

func TestBatchWriter_ConcurrentSets(t *testing.T) {
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 5,
		MaxItems: 10,
		MaxTime:  50 * time.Millisecond,
	})

	callCount := 0
	var mu sync.Mutex
	allData := make(map[string]string)

	writeFunc := func(ctx context.Context, data map[string]string) error {
		mu.Lock()
		callCount++
		for k, v := range data {
			allData[k] = v
		}
		mu.Unlock()
		return nil
	}

	writer := NewBatchWriter(config, writeFunc)
	defer writer.Close()

	// Launch concurrent writes
	var wg sync.WaitGroup
	errors := make([]error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := string(rune('a' + idx))
			value := "value-" + key
			errors[idx] = writer.Set(context.Background(), key, value)
		}(i)
	}

	wg.Wait()

	// Verify no errors
	for i, err := range errors {
		if err != nil {
			t.Errorf("unexpected error for index %d: %v", i, err)
		}
	}

	// Verify all data was written
	mu.Lock()
	defer mu.Unlock()

	if len(allData) != 10 {
		t.Errorf("expected 10 items written, got %d", len(allData))
	}

	// Should have batched into 2 calls (10 items / 5 min items)
	if callCount != 2 {
		t.Errorf("expected 2 batch calls, got %d", callCount)
	}
}

func TestBatchWriter_ContextCancellation(t *testing.T) {
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 5,
		MaxTime:  100 * time.Millisecond,
	})

	writeFunc := func(ctx context.Context, data map[string]string) error {
		// Simulate slow operation
		time.Sleep(50 * time.Millisecond)
		return nil
	}

	writer := NewBatchWriter(config, writeFunc)
	defer writer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := writer.Set(ctx, "key", "value")
	if err != context.Canceled {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}

func TestBatchWriter_BatchError(t *testing.T) {
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 1,
	})

	expectedErr := errors.New("database error")
	writeFunc := func(ctx context.Context, data map[string]string) error {
		return expectedErr
	}

	writer := NewBatchWriter(config, writeFunc)
	defer writer.Close()

	err := writer.Set(context.Background(), "key", "value")
	if err != expectedErr {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

func TestBatchWriter_DuplicateKeys(t *testing.T) {
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 3,
		MaxTime:  50 * time.Millisecond,
	})

	var writtenData map[string]string
	writeFunc := func(ctx context.Context, data map[string]string) error {
		writtenData = data
		return nil
	}

	writer := NewBatchWriter(config, writeFunc)
	defer writer.Close()

	// Write same key with different values
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()
			_ = writer.Set(context.Background(), "duplicate", string(rune('a'+val)))
		}(i)
	}

	wg.Wait()

	// Last write should win
	if len(writtenData) != 1 {
		t.Errorf("expected 1 unique key, got %d", len(writtenData))
	}
}

func TestBatchWriter_NilContext(t *testing.T) {
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 1,
	})

	called := false
	writeFunc := func(ctx context.Context, data map[string]string) error {
		called = true
		return nil
	}

	writer := NewBatchWriter(config, writeFunc)
	defer writer.Close()

	// Should not panic with nil context
	err := writer.Set(nil, "key", "value")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("write function was not called")
	}
}

func TestBatchWriter_MultipleClose(t *testing.T) {
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 1,
	})

	writeFunc := func(ctx context.Context, data map[string]string) error {
		return nil
	}

	writer := NewBatchWriter(config, writeFunc)

	// Multiple closes should not panic
	writer.Close()
	writer.Close()
}

// Benchmark tests
func BenchmarkBatchWriter_SingleSet(b *testing.B) {
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 1,
		MaxItems: 100,
		MaxTime:  10 * time.Millisecond,
	})

	writeFunc := func(ctx context.Context, data map[string]string) error {
		return nil
	}

	writer := NewBatchWriter(config, writeFunc)
	defer writer.Close()

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = writer.Set(ctx, "key", "value")
	}
}

func BenchmarkBatchWriter_ConcurrentSets(b *testing.B) {
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 10,
		MaxItems: 100,
		MaxTime:  10 * time.Millisecond,
	})

	writeFunc := func(ctx context.Context, data map[string]string) error {
		return nil
	}

	writer := NewBatchWriter(config, writeFunc)
	defer writer.Close()

	ctx := context.Background()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = writer.Set(ctx, "key", "value")
		}
	})
}
