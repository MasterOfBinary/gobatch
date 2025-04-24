package source

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestChannel_Read(t *testing.T) {
	t.Run("normal operation", func(t *testing.T) {
		// Setup
		testData := []interface{}{1, "two", 3.0, []int{4}}
		in := make(chan interface{}, len(testData))
		for _, item := range testData {
			in <- item
		}
		close(in)

		source := &Channel{Input: in}
		ctx := context.Background()

		// Execute
		out, errs := source.Read(ctx)

		// Collect results
		var results []interface{}
		var errors []error

		// Collect all data first
		for item := range out {
			results = append(results, item)
		}

		// Then collect any errors (should be none)
		for err := range errs {
			errors = append(errors, err)
		}

		// Verify
		if len(errors) != 0 {
			t.Errorf("expected no errors, got %v", errors)
		}

		if len(results) != len(testData) {
			t.Errorf("expected %d items, got %d", len(testData), len(results))
		}

		for i, expected := range testData {
			if i >= len(results) {
				break
			}

			// Special handling for slices which can't be compared directly
			if reflect.TypeOf(expected).Kind() == reflect.Slice {
				if !reflect.DeepEqual(expected, results[i]) {
					t.Errorf("item %d: expected %v, got %v", i, expected, results[i])
				}
			} else if results[i] != expected {
				t.Errorf("item %d: expected %v, got %v", i, expected, results[i])
			}
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		// Setup an infinite channel that would hang without cancellation
		in := make(chan interface{})
		defer close(in)

		source := &Channel{Input: in}
		ctx, cancel := context.WithCancel(context.Background())

		// Execute
		out, errs := source.Read(ctx)

		// Cancel immediately
		cancel()

		// Both channels should close soon
		timeout := time.After(100 * time.Millisecond)

		select {
		case _, ok := <-out:
			if ok {
				// Read anything available
				for range out {
				}
			}
		case <-timeout:
			t.Fatal("data channel didn't close after context cancellation")
		}

		select {
		case _, ok := <-errs:
			if ok {
				// Read anything available
				for range errs {
				}
			}
		case <-timeout:
			t.Fatal("error channel didn't close after context cancellation")
		}
	})

	t.Run("handles nil input", func(t *testing.T) {
		// Setup source with nil input
		source := &Channel{Input: nil}
		ctx := context.Background()

		// Execute
		out, errs := source.Read(ctx)

		// Both channels should close quickly
		timeout := time.After(100 * time.Millisecond)

		select {
		case _, ok := <-out:
			if ok {
				// Read anything available
				for range out {
				}
			}
		case <-timeout:
			t.Fatal("data channel didn't close with nil input")
		}

		select {
		case _, ok := <-errs:
			if ok {
				// Read anything available
				for range errs {
				}
			}
		case <-timeout:
			t.Fatal("error channel didn't close with nil input")
		}
	})

	t.Run("respects buffer size", func(t *testing.T) {
		bufSize := 5
		source := &Channel{
			Input:      make(chan interface{}),
			BufferSize: bufSize,
		}

		ctx := context.Background()
		out, _ := source.Read(ctx)

		// This assertion relies on implementation details (buffer size),
		// so it might need adjustment if implementation changes
		if cap(out) != bufSize {
			t.Errorf("expected buffer size %d, got %d", bufSize, cap(out))
		}
	})
}
