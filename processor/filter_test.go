package processor

import (
	"context"
	"testing"

	"github.com/MasterOfBinary/gobatch/batch"
)

func TestFilter_Process(t *testing.T) {
	t.Run("keeps items matching predicate", func(t *testing.T) {
		// Setup - keep only even-numbered IDs
		processor := &Filter{
			Predicate: func(item *batch.Item) bool {
				return item.ID%2 == 0 // Keep even IDs
			},
		}

		items := []*batch.Item{
			{ID: 1, Data: "odd"},
			{ID: 2, Data: "even"},
			{ID: 3, Data: "odd"},
			{ID: 4, Data: "even"},
		}

		// Execute
		ctx := context.Background()
		result, err := processor.Process(ctx, items)

		// Verify
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if len(result) != 2 {
			t.Errorf("expected 2 items, got %d", len(result))
		}

		for _, item := range result {
			if item.ID%2 != 0 {
				t.Errorf("unexpected odd ID: %d", item.ID)
			}
		}
	})

	t.Run("inverts predicate with InvertMatch", func(t *testing.T) {
		// Setup - remove even-numbered IDs (keep odd)
		processor := &Filter{
			Predicate: func(item *batch.Item) bool {
				return item.ID%2 == 0 // Matches even IDs
			},
			InvertMatch: true, // But we'll invert to keep odd IDs
		}

		items := []*batch.Item{
			{ID: 1, Data: "odd"},
			{ID: 2, Data: "even"},
			{ID: 3, Data: "odd"},
			{ID: 4, Data: "even"},
		}

		// Execute
		ctx := context.Background()
		result, _ := processor.Process(ctx, items)

		// Verify
		if len(result) != 2 {
			t.Errorf("expected 2 items, got %d", len(result))
		}

		for _, item := range result {
			if item.ID%2 == 0 {
				t.Errorf("unexpected even ID: %d", item.ID)
			}
		}
	})

	t.Run("handles nil predicate", func(t *testing.T) {
		// Setup - nil predicate should pass all items through
		processor := &Filter{
			Predicate: nil,
		}

		items := []*batch.Item{
			{ID: 1, Data: "item1"},
			{ID: 2, Data: "item2"},
		}

		// Execute
		ctx := context.Background()
		result, _ := processor.Process(ctx, items)

		// Verify - should have all items
		if len(result) != len(items) {
			t.Errorf("expected %d items, got %d", len(items), len(result))
		}
	})

	t.Run("handles empty items slice", func(t *testing.T) {
		// Setup
		processor := &Filter{
			Predicate: func(item *batch.Item) bool {
				return true // Keep all
			},
		}

		// Execute
		ctx := context.Background()
		result, err := processor.Process(ctx, []*batch.Item{})

		// Verify
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if len(result) != 0 {
			t.Errorf("expected empty slice, got %d items", len(result))
		}
	})

	t.Run("can filter all items out", func(t *testing.T) {
		// Setup - filter that rejects everything
		processor := &Filter{
			Predicate: func(item *batch.Item) bool {
				return false // Reject all
			},
		}

		items := []*batch.Item{
			{ID: 1, Data: "item1"},
			{ID: 2, Data: "item2"},
		}

		// Execute
		ctx := context.Background()
		result, _ := processor.Process(ctx, items)

		// Verify - should have no items
		if len(result) != 0 {
			t.Errorf("expected empty slice, got %d items", len(result))
		}
	})

	t.Run("filters based on Data field", func(t *testing.T) {
		// Setup - keep only items with string Data
		processor := &Filter{
			Predicate: func(item *batch.Item) bool {
				_, isString := item.Data.(string)
				return isString
			},
		}

		items := []*batch.Item{
			{ID: 1, Data: "string data"},    // keep
			{ID: 2, Data: 42},               // filter out
			{ID: 3, Data: "another string"}, // keep
			{ID: 4, Data: []int{1, 2, 3}},   // filter out
		}

		// Execute
		ctx := context.Background()
		result, _ := processor.Process(ctx, items)

		// Verify - should have only the string items
		if len(result) != 2 {
			t.Errorf("expected 2 items, got %d", len(result))
		}

		for _, item := range result {
			if _, isString := item.Data.(string); !isString {
				t.Errorf("item %d: expected string data, got %T", item.ID, item.Data)
			}
		}
	})
}
