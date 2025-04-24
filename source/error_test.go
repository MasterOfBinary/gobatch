package source

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestError_Read(t *testing.T) {
	t.Run("forwards errors correctly", func(t *testing.T) {
		// Setup
		testErrors := []error{
			errors.New("error 1"),
			errors.New("error 2"),
			errors.New("error 3"),
		}

		errCh := make(chan error, len(testErrors))
		for _, err := range testErrors {
			errCh <- err
		}
		close(errCh)

		source := &Error{Errs: errCh}
		ctx := context.Background()

		// Execute
		out, errs := source.Read(ctx)

		// Collect results
		var data []interface{}
		var collectedErrors []error

		// Create a timeout to prevent hanging if channels don't close
		timeout := time.After(100 * time.Millisecond)

		// Check that data channel closes (it should be empty)
		select {
		case item, ok := <-out:
			if ok {
				data = append(data, item)
				for item := range out {
					data = append(data, item)
				}
			}
		case <-timeout:
			t.Fatal("data channel didn't close")
		}

		// Collect all errors
		errTimeout := time.After(100 * time.Millisecond)
	errLoop:
		for {
			select {
			case err, ok := <-errs:
				if !ok {
					break errLoop
				}
				collectedErrors = append(collectedErrors, err)
			case <-errTimeout:
				t.Fatal("error channel didn't close")
			}
		}

		// Verify
		if len(data) != 0 {
			t.Errorf("expected no data, got %v", data)
		}

		if len(collectedErrors) != len(testErrors) {
			t.Errorf("expected %d errors, got %d", len(testErrors), len(collectedErrors))
		}

		for i, expected := range testErrors {
			if i >= len(collectedErrors) {
				break
			}
			if collectedErrors[i].Error() != expected.Error() {
				t.Errorf("error %d: expected %v, got %v", i, expected, collectedErrors[i])
			}
		}
	})

	t.Run("ignores nil errors", func(t *testing.T) {
		// Setup
		errCh := make(chan error, 3)
		errCh <- errors.New("real error")
		errCh <- nil // Should be ignored
		errCh <- errors.New("another error")
		close(errCh)

		source := &Error{Errs: errCh}
		ctx := context.Background()

		// Execute
		_, errs := source.Read(ctx)

		// Collect errors
		var collectedErrors []error
		for err := range errs {
			collectedErrors = append(collectedErrors, err)
		}

		// Verify
		if len(collectedErrors) != 2 {
			t.Errorf("expected 2 errors (nil error should be ignored), got %d", len(collectedErrors))
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		// Setup an infinite error channel that would hang without cancellation
		errCh := make(chan error)
		defer close(errCh)

		source := &Error{Errs: errCh}
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

	t.Run("handles nil error channel", func(t *testing.T) {
		// Setup
		source := &Error{Errs: nil}
		ctx := context.Background()

		// Execute
		out, errs := source.Read(ctx)

		// Both channels should close quickly
		timeout := time.After(100 * time.Millisecond)

		select {
		case _, ok := <-out:
			if ok {
				for range out {
				}
			}
		case <-timeout:
			t.Fatal("data channel didn't close with nil error channel")
		}

		select {
		case _, ok := <-errs:
			if ok {
				for range errs {
				}
			}
		case <-timeout:
			t.Fatal("error channel didn't close with nil error channel")
		}
	})

	t.Run("respects buffer size", func(t *testing.T) {
		bufSize := 5
		source := &Error{
			Errs:       make(chan error),
			BufferSize: bufSize,
		}

		ctx := context.Background()
		_, errs := source.Read(ctx)

		// This assertion relies on implementation details (buffer size),
		// so it might need adjustment if implementation changes
		if cap(errs) != bufSize {
			t.Errorf("expected buffer size %d, got %d", bufSize, cap(errs))
		}
	})
}
