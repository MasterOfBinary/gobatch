package processor

import (
	"context"
	"errors"
	"testing"

	"github.com/MasterOfBinary/gobatch/batch"
)

func TestFilter_Process(t *testing.T) {
	t.Run("keeps items matching predicate", func(t *testing.T) {
		// Setup - keep only even-numbered IDs
		processor := &Filter[any]{
			Predicate: func(item *batch.Item[any]) bool {
				return item.ID%2 == 0 // Keep even IDs
			},
		}

		items := []*batch.Item[any]{
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
		processor := &Filter[any]{
			Predicate: func(item *batch.Item[any]) bool {
				return item.ID%2 == 0 // Matches even IDs
			},
			InvertMatch: true, // But we'll invert to keep odd IDs
		}

		items := []*batch.Item[any]{
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
		processor := &Filter[any]{
			Predicate: nil,
		}

		items := []*batch.Item[any]{
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
		processor := &Filter[any]{
			Predicate: func(item *batch.Item[any]) bool {
				return true // Keep all
			},
		}

		// Execute
		ctx := context.Background()
		result, err := processor.Process(ctx, []*batch.Item[any]{})

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
		processor := &Filter[any]{
			Predicate: func(item *batch.Item[any]) bool {
				return false // Reject all
			},
		}

		items := []*batch.Item[any]{
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
		processor := &Filter[any]{
			Predicate: func(item *batch.Item[any]) bool {
				_, isString := item.Data.(string)
				return isString
			},
		}

		items := []*batch.Item[any]{
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

// containsID reports whether the result slice contains an item with the given ID.
func containsID(result []*batch.Item[any], id uint64) bool {
	for _, item := range result {
		if item.ID == id {
			return true
		}
	}
	return false
}

func TestFilter_Process_PreservesErroredItems(t *testing.T) {
	errBoom := errors.New("boom")

	t.Run("errored item passes through even when predicate rejects it", func(t *testing.T) {
		// The predicate would filter the errored item out, which used to silently
		// drop its error before the batch engine could report it.
		processor := &Filter[any]{
			Predicate: func(item *batch.Item[any]) bool {
				return item.Error == nil // would exclude any errored item
			},
		}

		items := []*batch.Item[any]{
			{ID: 1, Data: "ok"},
			{ID: 2, Data: "failed", Error: errBoom},
		}

		ctx := context.Background()
		result, err := processor.Process(ctx, items)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !containsID(result, 2) {
			t.Fatalf("errored item (ID 2) was dropped; its error would be silently lost")
		}

		// Confirm the error is preserved unchanged on the passed-through item.
		for _, item := range result {
			if item.ID == 2 && !errors.Is(item.Error, errBoom) {
				t.Errorf("expected errored item to retain its error, got %v", item.Error)
			}
		}

		// The error-free item that the predicate keeps must still be present.
		if !containsID(result, 1) {
			t.Errorf("error-free matching item (ID 1) should be kept")
		}
	})

	t.Run("predicate still filters error-free items", func(t *testing.T) {
		// With one errored item passing through, the predicate must continue to
		// apply normally to the error-free items.
		processor := &Filter[any]{
			Predicate: func(item *batch.Item[any]) bool {
				return item.ID%2 == 0 // keep even IDs
			},
		}

		items := []*batch.Item[any]{
			{ID: 1, Data: "odd"},                     // filtered out
			{ID: 2, Data: "even"},                    // kept
			{ID: 3, Data: "errored", Error: errBoom}, // passes through despite odd ID
			{ID: 4, Data: "even"},                    // kept
		}

		ctx := context.Background()
		result, _ := processor.Process(ctx, items)

		if len(result) != 3 {
			t.Fatalf("expected 3 items (2 even + 1 errored), got %d", len(result))
		}
		if containsID(result, 1) {
			t.Errorf("odd error-free item (ID 1) should have been filtered out")
		}
		if !containsID(result, 3) {
			t.Errorf("errored item (ID 3) should pass through")
		}
	})

	t.Run("errored item passes through even with InvertMatch", func(t *testing.T) {
		// With InvertMatch the predicate keeps non-matching items. The errored item
		// must bypass the predicate entirely regardless of inversion.
		processor := &Filter[any]{
			Predicate: func(item *batch.Item[any]) bool {
				return true // inverted => would reject everything
			},
			InvertMatch: true,
		}

		items := []*batch.Item[any]{
			{ID: 1, Data: "ok"},                     // inverted predicate => dropped
			{ID: 2, Data: "failed", Error: errBoom}, // must pass through
		}

		ctx := context.Background()
		result, _ := processor.Process(ctx, items)

		if !containsID(result, 2) {
			t.Fatalf("errored item (ID 2) was dropped under InvertMatch; error would be lost")
		}
		if containsID(result, 1) {
			t.Errorf("error-free item (ID 1) should be dropped by inverted predicate")
		}
	})
}
