package processor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/MasterOfBinary/gobatch/batch"
)

func TestNewTransform(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		proc, err := NewTransform(TransformConfig{
			Func: func(data interface{}) (interface{}, error) {
				return data, nil
			},
			ContinueOnError: true,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if proc.Func == nil {
			t.Error("transform function not set")
		}

		if !proc.ContinueOnError {
			t.Error("expected ContinueOnError=true")
		}
	})

	t.Run("nil transform function", func(t *testing.T) {
		_, err := NewTransform(TransformConfig{
			Func: nil,
		})

		if err == nil {
			t.Fatal("expected error for nil transform function")
		}

		if !strings.Contains(err.Error(), "transformation function cannot be nil") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("transform with error handling", func(t *testing.T) {
		// Test with ContinueOnError = false
		proc, err := NewTransform(TransformConfig{
			Func: func(data interface{}) (interface{}, error) {
				if data == "error" {
					return nil, errors.New("transform error")
				}
				return data, nil
			},
			ContinueOnError: false,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		items := []*batch.Item{
			{ID: 1, Data: "ok"},
			{ID: 2, Data: "error"},
			{ID: 3, Data: "ok"},
		}

		_, procErr := proc.Process(context.Background(), items)
		if procErr == nil {
			t.Error("expected processing error")
		}

		// First item should be unmodified
		if items[0].Error != nil {
			t.Error("first item should not have error")
		}

		// Second item should have error
		if items[1].Error == nil {
			t.Error("second item should have error")
		}
	})
}

func TestNewFilter(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		proc, err := NewFilter(FilterConfig{
			Predicate: func(item *batch.Item) bool {
				return true
			},
			InvertMatch: false,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if proc.Predicate == nil {
			t.Error("predicate function not set")
		}

		if proc.InvertMatch {
			t.Error("expected InvertMatch=false")
		}
	})

	t.Run("nil predicate function", func(t *testing.T) {
		_, err := NewFilter(FilterConfig{
			Predicate: nil,
		})

		if err == nil {
			t.Fatal("expected error for nil predicate function")
		}

		if !strings.Contains(err.Error(), "predicate function cannot be nil") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("filter functionality", func(t *testing.T) {
		proc, err := NewFilter(FilterConfig{
			Predicate: func(item *batch.Item) bool {
				// Keep only even numbers
				if num, ok := item.Data.(int); ok {
					return num%2 == 0
				}
				return false
			},
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		items := []*batch.Item{
			{ID: 1, Data: 1},
			{ID: 2, Data: 2},
			{ID: 3, Data: 3},
			{ID: 4, Data: 4},
		}

		filtered, _ := proc.Process(context.Background(), items)

		if len(filtered) != 2 {
			t.Errorf("expected 2 items, got %d", len(filtered))
		}

		// Check that only even numbers remain
		for _, item := range filtered {
			if num, ok := item.Data.(int); ok {
				if num%2 != 0 {
					t.Errorf("unexpected odd number: %d", num)
				}
			}
		}
	})

	t.Run("filter with invert match", func(t *testing.T) {
		proc, err := NewFilter(FilterConfig{
			Predicate: func(item *batch.Item) bool {
				// Match even numbers
				if num, ok := item.Data.(int); ok {
					return num%2 == 0
				}
				return false
			},
			InvertMatch: true, // Keep odd numbers instead
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		items := []*batch.Item{
			{ID: 1, Data: 1},
			{ID: 2, Data: 2},
			{ID: 3, Data: 3},
			{ID: 4, Data: 4},
		}

		filtered, _ := proc.Process(context.Background(), items)

		if len(filtered) != 2 {
			t.Errorf("expected 2 items, got %d", len(filtered))
		}

		// Check that only odd numbers remain (inverted)
		for _, item := range filtered {
			if num, ok := item.Data.(int); ok {
				if num%2 == 0 {
					t.Errorf("unexpected even number: %d", num)
				}
			}
		}
	})
}

func TestNewError(t *testing.T) {
	t.Run("with error and zero fail fraction", func(t *testing.T) {
		testErr := errors.New("test error")
		proc := NewError(ErrorConfig{
			Err: testErr,
			// FailFraction not set, defaults to 0
		})

		if proc.Err != testErr {
			t.Error("error not set correctly")
		}

		if proc.FailFraction != 0.0 {
			t.Errorf("expected FailFraction=0.0 (default), got %f", proc.FailFraction)
		}
	})

	t.Run("with custom fail fraction", func(t *testing.T) {
		testErr := errors.New("test error")
		proc := NewError(ErrorConfig{
			Err:          testErr,
			FailFraction: 0.5,
		})

		if proc.FailFraction != 0.5 {
			t.Errorf("expected FailFraction=0.5, got %f", proc.FailFraction)
		}
	})

	t.Run("nil error with fail fraction", func(t *testing.T) {
		proc := NewError(ErrorConfig{
			Err:          nil,
			FailFraction: 1.0,
		})

		if proc.Err != nil {
			t.Error("expected nil error")
		}

		// Process should use default error message
		items := []*batch.Item{{ID: 1, Data: "test"}}
		result, err := proc.Process(context.Background(), items)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if len(result) != 1 {
			t.Error("items should pass through")
		}

		if result[0].Error == nil {
			t.Error("item should have default error")
		} else if result[0].Error.Error() != "processor error" {
			t.Errorf("expected default error message, got: %v", result[0].Error)
		}
	})

	t.Run("zero fail fraction", func(t *testing.T) {
		proc := NewError(ErrorConfig{
			Err:          errors.New("test error"),
			FailFraction: 0.0,
		})

		// Process should not apply errors
		items := []*batch.Item{{ID: 1, Data: "test"}}
		result, err := proc.Process(context.Background(), items)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if result[0].Error != nil {
			t.Error("item should not have error with FailFraction=0")
		}
	})
}

func TestNewNil(t *testing.T) {
	proc := NewNil()

	if proc == nil {
		t.Fatal("NewNil returned nil")
	}

	// Test that it passes items through unchanged
	items := []*batch.Item{
		{ID: 1, Data: "test1"},
		{ID: 2, Data: "test2"},
	}

	result, err := proc.Process(context.Background(), items)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(result) != len(items) {
		t.Errorf("expected %d items, got %d", len(items), len(result))
	}

	// Items should be unchanged
	for i, item := range result {
		if item.ID != items[i].ID || item.Data != items[i].Data {
			t.Error("items were modified")
		}
	}
}
