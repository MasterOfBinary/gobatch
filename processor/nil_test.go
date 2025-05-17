package processor

import (
	"context"
	"testing"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
)

func TestNil_Process(t *testing.T) {
	t.Run("returns items unchanged with zero duration", func(t *testing.T) {
		// Setup
		processor := &Nil{Duration: 0}

		items := []*batch.Item{
			{ID: 1, Data: "item1"},
			{ID: 2, Data: "item2"},
		}

		// Execute
		ctx := context.Background()
		start := time.Now()
		processedItems, err := processor.Process(ctx, items)
		elapsed := time.Since(start)

		// Verify
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Should return immediately with zero duration
		if elapsed > 10*time.Millisecond {
			t.Errorf("expected immediate return, took %v", elapsed)
		}

		if len(processedItems) != len(items) {
			t.Errorf("expected %d items, got %d", len(items), len(processedItems))
		}

		// Items should be unchanged
		for i, item := range processedItems {
			if item.ID != items[i].ID {
				t.Errorf("item %d: ID changed", i)
			}
			if item.Data != items[i].Data {
				t.Errorf("item %d: Data changed", i)
			}
			if item.Error != nil {
				t.Errorf("item %d: unexpected error: %v", i, item.Error)
			}
		}
	})

	t.Run("waits for the specified duration", func(t *testing.T) {
		// Setup
		duration := 50 * time.Millisecond
		processor := &Nil{Duration: duration}

		items := []*batch.Item{
			{ID: 1, Data: "test"},
		}

		// Execute
		ctx := context.Background()
		start := time.Now()
		processedItems, _ := processor.Process(ctx, items)
		elapsed := time.Since(start)

		// Verify
		if elapsed < duration {
			t.Errorf("should have waited at least %v, only waited %v", duration, elapsed)
		}

		// Items should be unchanged
		if processedItems[0].Error != nil {
			t.Errorf("unexpected error: %v", processedItems[0].Error)
		}
	})

	t.Run("handles empty items slice", func(t *testing.T) {
		// Setup
		processor := &Nil{
			Duration: 50 * time.Millisecond,
		}

		// Execute
		ctx := context.Background()
		start := time.Now()
		processedItems, err := processor.Process(ctx, []*batch.Item{})
		elapsed := time.Since(start)

		// Verify
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Should return immediately with empty slice
		if elapsed > 10*time.Millisecond {
			t.Errorf("expected immediate return with empty slice, took %v", elapsed)
		}

		if len(processedItems) != 0 {
			t.Errorf("expected empty slice, got %v items", len(processedItems))
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		// Setup
		processor := &Nil{
			Duration:      1 * time.Second, // Long enough to not complete naturally
			MarkCancelled: true,
		}

		items := []*batch.Item{
			{ID: 1, Data: "item1"},
			{ID: 2, Data: "item2"},
		}

		// Execute with a context that we'll cancel immediately
		ctx, cancel := context.WithCancel(context.Background())

		// Start processing in a goroutine
		resultCh := make(chan struct {
			items []*batch.Item
			err   error
		})

		go func() {
			items, err := processor.Process(ctx, items)
			resultCh <- struct {
				items []*batch.Item
				err   error
			}{items, err}
		}()

		// Cancel immediately
		cancel()

		// Get result
		result := <-resultCh

		// Verify
		if result.err != nil {
			t.Errorf("unexpected error: %v", result.err)
		}

		// Should mark items with context cancellation error
		for i, item := range result.items {
			if item.Error == nil {
				t.Errorf("item %d: expected context cancellation error, got nil", i)
			} else if item.Error != context.Canceled {
				t.Errorf("item %d: expected %v, got %v", i, context.Canceled, item.Error)
			}
		}
	})

	t.Run("respects MarkCancelled=false setting", func(t *testing.T) {
		// Setup - this time don't mark items as canceled
		processor := &Nil{
			Duration:      1 * time.Second,
			MarkCancelled: false,
		}

		items := []*batch.Item{
			{ID: 1, Data: "test"},
		}

		// Execute with canceled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		processedItems, _ := processor.Process(ctx, items)

		// Verify - should NOT mark items with error
		for i, item := range processedItems {
			if item.Error != nil {
				t.Errorf("item %d: expected nil error with MarkCancelled=false, got %v", i, item.Error)
			}
		}
	})
}
