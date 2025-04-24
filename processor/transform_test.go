package processor

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/MasterOfBinary/gobatch/batch"
)

func TestTransform_Process(t *testing.T) {
	t.Run("transforms data successfully", func(t *testing.T) {
		// Setup - double all integer values
		processor := &Transform{
			Func: func(data interface{}) (interface{}, error) {
				if val, ok := data.(int); ok {
					return val * 2, nil
				}
				return data, nil // Pass through non-integers
			},
		}

		items := []*batch.Item{
			{ID: 1, Data: 5},
			{ID: 2, Data: 10},
			{ID: 3, Data: "not an int"},
		}

		// Execute
		ctx := context.Background()
		result, err := processor.Process(ctx, items)

		// Verify
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if len(result) != len(items) {
			t.Errorf("expected %d items, got %d", len(items), len(result))
		}

		if val, ok := result[0].Data.(int); !ok || val != 10 {
			t.Errorf("item 0: expected 10, got %v", result[0].Data)
		}

		if val, ok := result[1].Data.(int); !ok || val != 20 {
			t.Errorf("item 1: expected 20, got %v", result[1].Data)
		}

		// Non-integer should be unchanged
		if val, ok := result[2].Data.(string); !ok || val != "not an int" {
			t.Errorf("item 2: expected 'not an int', got %v", result[2].Data)
		}
	})

	t.Run("handles transformation errors with ContinueOnError=true", func(t *testing.T) {
		// Setup - convert to int but fail on non-integers
		processor := &Transform{
			Func: func(data interface{}) (interface{}, error) {
				if str, ok := data.(string); ok {
					val, err := strconv.Atoi(str)
					if err != nil {
						return nil, fmt.Errorf("not a number: %s", str)
					}
					return val, nil
				}
				return data, nil
			},
			ContinueOnError: true, // Continue after errors
		}

		items := []*batch.Item{
			{ID: 1, Data: "123"},       // Should convert to 123
			{ID: 2, Data: "not a num"}, // Should error
			{ID: 3, Data: "456"},       // Should convert to 456
		}

		// Execute
		ctx := context.Background()
		result, err := processor.Process(ctx, items)

		// Verify
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// First item should be transformed
		if val, ok := result[0].Data.(int); !ok || val != 123 {
			t.Errorf("item 0: expected 123, got %v", result[0].Data)
		}

		// Second item should have error but data unchanged
		if result[1].Error == nil {
			t.Errorf("item 1: expected error, got nil")
		}
		if result[1].Data != "not a num" {
			t.Errorf("item 1: data should be unchanged, got %v", result[1].Data)
		}

		// Third item should be transformed
		if val, ok := result[2].Data.(int); !ok || val != 456 {
			t.Errorf("item 2: expected 456, got %v", result[2].Data)
		}
	})

	t.Run("stops on first error with ContinueOnError=false", func(t *testing.T) {
		// Setup
		testErr := errors.New("transformation failed")
		processor := &Transform{
			Func: func(data interface{}) (interface{}, error) {
				if val, ok := data.(int); ok {
					if val == 2 {
						return nil, testErr
					}
					return val * 2, nil
				}
				return data, nil
			},
			ContinueOnError: false, // Stop on first error
		}

		items := []*batch.Item{
			{ID: 1, Data: 1}, // Should be doubled to 2
			{ID: 2, Data: 2}, // Should cause error
			{ID: 3, Data: 3}, // Should not be processed
		}

		// Execute
		ctx := context.Background()
		result, err := processor.Process(ctx, items)

		// Verify
		if err != testErr {
			t.Errorf("expected specific error, got: %v", err)
		}

		// First item should be transformed
		if val, ok := result[0].Data.(int); !ok || val != 2 {
			t.Errorf("item 0: expected 2, got %v", result[0].Data)
		}

		// Second item should have error set
		if result[1].Error != testErr {
			t.Errorf("item 1: expected specific error, got %v", result[1].Error)
		}

		// Third item should be unchanged
		if val, ok := result[2].Data.(int); !ok || val != 3 {
			t.Errorf("item 2: expected 3 (unchanged), got %v", result[2].Data)
		}
	})

	t.Run("skips items with existing errors", func(t *testing.T) {
		// Setup
		processor := &Transform{
			Func: func(data interface{}) (interface{}, error) {
				if val, ok := data.(int); ok {
					return val * 2, nil
				}
				return data, nil
			},
		}

		existingErr := errors.New("existing error")
		items := []*batch.Item{
			{ID: 1, Data: 10, Error: existingErr}, // Already has error, should be skipped
			{ID: 2, Data: 20},                     // Should be doubled
		}

		// Execute
		ctx := context.Background()
		result, _ := processor.Process(ctx, items)

		// Verify - first item should be unchanged
		if result[0].Data != 10 || result[0].Error != existingErr {
			t.Errorf("item 0: data or error changed when it shouldn't have")
		}

		// Second item should be transformed
		if val, ok := result[1].Data.(int); !ok || val != 40 {
			t.Errorf("item 1: expected 40, got %v", result[1].Data)
		}
	})

	t.Run("handles nil transform function", func(t *testing.T) {
		// Setup - nil function should pass through
		processor := &Transform{
			Func: nil,
		}

		items := []*batch.Item{
			{ID: 1, Data: "test"},
		}

		// Execute
		ctx := context.Background()
		result, err := processor.Process(ctx, items)

		// Verify
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if result[0].Data != "test" {
			t.Errorf("expected unchanged data, got %v", result[0].Data)
		}
	})

	t.Run("handles empty items slice", func(t *testing.T) {
		// Setup
		processor := &Transform{
			Func: func(data interface{}) (interface{}, error) {
				return "transformed", nil
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
}
