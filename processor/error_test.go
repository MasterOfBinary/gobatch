package processor

import (
	"context"
	"errors"
	"testing"

	"github.com/MasterOfBinary/gobatch/batch"
)

func TestError_Process(t *testing.T) {
	t.Run("applies error to all items with default settings", func(t *testing.T) {
		// Setup
		customErr := errors.New("custom test error")
		processor := &Error{
			Err:          customErr,
			FailFraction: 1.0, // Default - apply to all
		}

		items := []*batch.Item{
			{ID: 1, Data: "item1"},
			{ID: 2, Data: "item2"},
			{ID: 3, Data: "item3"},
		}

		// Execute
		ctx := context.Background()
		processedItems, err := processor.Process(ctx, items)

		// Verify
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if len(processedItems) != len(items) {
			t.Errorf("expected %d items, got %d", len(items), len(processedItems))
		}

		for i, item := range processedItems {
			if item.Error == nil {
				t.Errorf("item %d: expected error, got nil", i)
			} else if item.Error != customErr {
				t.Errorf("item %d: expected error %v, got %v", i, customErr, item.Error)
			}

			// Data and ID should remain unchanged
			if item.ID != items[i].ID {
				t.Errorf("item %d: ID changed from %v to %v", i, items[i].ID, item.ID)
			}
			if item.Data != items[i].Data {
				t.Errorf("item %d: Data changed from %v to %v", i, items[i].Data, item.Data)
			}
		}
	})

	t.Run("uses default error when nil provided", func(t *testing.T) {
		// Setup
		processor := &Error{
			Err:          nil, // Should use default
			FailFraction: 1.0,
		}

		items := []*batch.Item{
			{ID: 1, Data: "test"},
		}

		// Execute
		ctx := context.Background()
		processedItems, _ := processor.Process(ctx, items)

		// Verify
		if processedItems[0].Error == nil {
			t.Error("expected default error, got nil")
		} else if processedItems[0].Error.Error() != "processor error" {
			t.Errorf("expected 'processor error', got %v", processedItems[0].Error)
		}
	})

	t.Run("handles empty items slice", func(t *testing.T) {
		// Setup
		processor := &Error{
			Err:          errors.New("test"),
			FailFraction: 1.0,
		}

		// Execute
		ctx := context.Background()
		processedItems, err := processor.Process(ctx, []*batch.Item{})

		// Verify
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if len(processedItems) != 0 {
			t.Errorf("expected empty slice, got %v items", len(processedItems))
		}
	})

	t.Run("applies error to fraction of items", func(t *testing.T) {
		// Setup
		customErr := errors.New("partial error")
		processor := &Error{
			Err:          customErr,
			FailFraction: 0.5, // Apply to approximately half
		}

		// Create 10 items - about 5 should have errors
		items := make([]*batch.Item, 10)
		for i := 0; i < 10; i++ {
			items[i] = &batch.Item{ID: uint64(i), Data: i}
		}

		// Execute
		ctx := context.Background()
		processedItems, err := processor.Process(ctx, items)

		// Verify
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		errorCount := 0
		for _, item := range processedItems {
			if item.Error != nil {
				errorCount++
				if item.Error != customErr {
					t.Errorf("wrong error: expected %v, got %v", customErr, item.Error)
				}
			}
		}

		// With 10 items and FailFraction=0.5, we expect every other item to have an error
		// Specifically items with index 0, 2, 4, 6, 8 for a total of 5 errors
		expectedErrorCount := 5
		if errorCount != expectedErrorCount {
			t.Errorf("expected %d errors, got %d", expectedErrorCount, errorCount)
		}
	})

	t.Run("applies no errors when fail fraction is zero", func(t *testing.T) {
		// Setup
		processor := &Error{
			Err:          errors.New("test"),
			FailFraction: 0, // No errors
		}

		items := []*batch.Item{
			{ID: 1, Data: "item1"},
			{ID: 2, Data: "item2"},
		}

		// Execute
		ctx := context.Background()
		processedItems, _ := processor.Process(ctx, items)

		// Verify - no items should have errors
		for i, item := range processedItems {
			if item.Error != nil {
				t.Errorf("item %d: expected nil error, got %v", i, item.Error)
			}
		}
	})
}
