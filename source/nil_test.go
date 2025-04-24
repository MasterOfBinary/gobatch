package source

import (
	"context"
	"testing"
	"time"
)

func TestNil_Read(t *testing.T) {
	t.Run("closes immediately with zero duration", func(t *testing.T) {
		// Setup
		source := &Nil{Duration: 0}
		ctx := context.Background()

		// Execute
		out, errs := source.Read(ctx)

		// Both channels should close immediately
		timeout := time.After(50 * time.Millisecond)

		select {
		case _, ok := <-out:
			if ok {
				for range out {
				}
			}
		case <-timeout:
			t.Fatal("data channel didn't close with zero duration")
		}

		select {
		case _, ok := <-errs:
			if ok {
				for range errs {
				}
			}
		case <-timeout:
			t.Fatal("error channel didn't close with zero duration")
		}
	})

	t.Run("closes after specified duration", func(t *testing.T) {
		// Setup
		duration := 50 * time.Millisecond
		source := &Nil{Duration: duration}
		ctx := context.Background()

		// Execute
		out, errs := source.Read(ctx)

		// Verify channels stay open before duration elapses
		timeoutEarly := time.After(duration / 2)

		select {
		case _, ok := <-out:
			if !ok {
				t.Fatal("data channel closed too early")
			}
		case <-timeoutEarly:
			// This is expected - timeout before channels close
		}

		// Verify channels close after duration elapses
		timeoutLate := time.After(duration * 2)

		select {
		case _, ok := <-out:
			if ok {
				for range out {
				}
			}
		case <-timeoutLate:
			t.Fatal("data channel didn't close after duration")
		}

		select {
		case _, ok := <-errs:
			if ok {
				for range errs {
				}
			}
		case <-timeoutLate:
			t.Fatal("error channel didn't close after duration")
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		// Setup
		source := &Nil{Duration: 10 * time.Second} // Long enough to not complete
		ctx, cancel := context.WithCancel(context.Background())

		// Execute
		out, errs := source.Read(ctx)

		// Cancel immediately
		cancel()

		// Both channels should close soon despite long duration
		timeout := time.After(100 * time.Millisecond)

		select {
		case _, ok := <-out:
			if ok {
				for range out {
				}
			}
		case <-timeout:
			t.Fatal("data channel didn't close after context cancellation")
		}

		select {
		case _, ok := <-errs:
			if ok {
				for range errs {
				}
			}
		case <-timeout:
			t.Fatal("error channel didn't close after context cancellation")
		}
	})

	t.Run("negative duration treated as zero", func(t *testing.T) {
		// Setup
		source := &Nil{Duration: -10 * time.Millisecond}
		ctx := context.Background()

		// Execute
		out, errs := source.Read(ctx)

		// Both channels should close quickly
		timeout := time.After(50 * time.Millisecond)

		select {
		case _, ok := <-out:
			if ok {
				for range out {
				}
			}
		case <-timeout:
			t.Fatal("data channel didn't close with negative duration")
		}

		select {
		case _, ok := <-errs:
			if ok {
				for range errs {
				}
			}
		case <-timeout:
			t.Fatal("error channel didn't close with negative duration")
		}
	})

	t.Run("emits no data or errors", func(t *testing.T) {
		// Setup
		source := &Nil{Duration: 10 * time.Millisecond}
		ctx := context.Background()

		// Execute
		out, errs := source.Read(ctx)

		// Collect results
		var data []interface{}
		var errors []error

		// Wait for channels to close
		for item := range out {
			data = append(data, item)
		}

		for err := range errs {
			errors = append(errors, err)
		}

		// Verify
		if len(data) != 0 {
			t.Errorf("expected no data, got %v", data)
		}

		if len(errors) != 0 {
			t.Errorf("expected no errors, got %v", errors)
		}
	})
}
