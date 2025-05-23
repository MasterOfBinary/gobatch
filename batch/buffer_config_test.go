package batch

import (
	"context"
	"strings"
	"testing"
)

// testSourceFunc test helper - simple source implementation
type testSourceFunc func(ctx context.Context) (<-chan interface{}, <-chan error)

func (f testSourceFunc) Read(ctx context.Context) (<-chan interface{}, <-chan error) {
	return f(ctx)
}

func TestBatch_WithBufferConfig(t *testing.T) {
	t.Run("custom buffer sizes", func(t *testing.T) {
		customConfig := BufferConfig{
			ItemBufferSize:  500,
			IDBufferSize:    600,
			ErrorBufferSize: 200,
		}

		b := New(nil).WithBufferConfig(customConfig)

		// Verify the config was set
		if b.bufferConfig.ItemBufferSize != 500 {
			t.Errorf("expected ItemBufferSize=500, got %d", b.bufferConfig.ItemBufferSize)
		}
		if b.bufferConfig.IDBufferSize != 600 {
			t.Errorf("expected IDBufferSize=600, got %d", b.bufferConfig.IDBufferSize)
		}
		if b.bufferConfig.ErrorBufferSize != 200 {
			t.Errorf("expected ErrorBufferSize=200, got %d", b.bufferConfig.ErrorBufferSize)
		}
	})

	t.Run("zero values use defaults", func(t *testing.T) {
		// Create batch with zero buffer config
		b := New(nil)

		// Create a simple source
		src := testSourceFunc(func(ctx context.Context) (<-chan interface{}, <-chan error) {
			out := make(chan interface{})
			errs := make(chan error)
			go func() {
				defer close(out)
				defer close(errs)
				out <- "test"
			}()
			return out, errs
		})

		// Start processing to trigger channel creation
		errs := b.Go(context.Background(), src)
		IgnoreErrors(errs)
		<-b.Done()

		// Can't directly test channel buffer sizes, but test should not panic
	})

	t.Run("negative values use defaults", func(t *testing.T) {
		customConfig := BufferConfig{
			ItemBufferSize:  -1,
			IDBufferSize:    -1,
			ErrorBufferSize: -1,
		}

		b := New(nil).WithBufferConfig(customConfig)

		// Create a simple source
		src := testSourceFunc(func(ctx context.Context) (<-chan interface{}, <-chan error) {
			out := make(chan interface{})
			errs := make(chan error)
			go func() {
				defer close(out)
				defer close(errs)
				out <- "test"
			}()
			return out, errs
		})

		// Start processing - should use default buffer sizes
		errs := b.Go(context.Background(), src)
		IgnoreErrors(errs)
		<-b.Done()

		// Test should not panic
	})

	t.Run("panic if called after Go", func(t *testing.T) {
		b := New(nil)

		// Create a simple source
		src := testSourceFunc(func(ctx context.Context) (<-chan interface{}, <-chan error) {
			out := make(chan interface{})
			errs := make(chan error)
			go func() {
				defer close(out)
				defer close(errs)
				out <- "test"
			}()
			return out, errs
		})

		// Start batch processing
		errs := b.Go(context.Background(), src)

		// Should panic when trying to set buffer config after Go
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic when calling WithBufferConfig after Go")
			} else if msg, ok := r.(string); ok {
				if !strings.Contains(msg, "WithBufferConfig cannot be called after Go() has started") {
					t.Errorf("unexpected panic message: %s", msg)
				}
			}
		}()

		b.WithBufferConfig(BufferConfig{
			ItemBufferSize: 500,
		})

		// Clean up
		IgnoreErrors(errs)
		<-b.Done()
	})

	t.Run("thread safe with Go", func(t *testing.T) {
		b := New(nil)

		// Try to call WithBufferConfig and Go concurrently
		// One should succeed, one should panic
		done := make(chan bool, 2)

		go func() {
			defer func() {
				_ = recover() // Ignore panic
				done <- true
			}()
			b.WithBufferConfig(BufferConfig{
				ItemBufferSize: 500,
			})
		}()

		go func() {
			defer func() {
				done <- true
			}()
			src := testSourceFunc(func(ctx context.Context) (<-chan interface{}, <-chan error) {
				out := make(chan interface{})
				errs := make(chan error)
				close(out)
				close(errs)
				return out, errs
			})
			errs := b.Go(context.Background(), src)
			IgnoreErrors(errs)
		}()

		// Wait for both goroutines
		<-done
		<-done

		// Wait for batch to complete
		<-b.Done()
	})
}

func TestDefaultConstants(t *testing.T) {
	// Verify default constants are reasonable
	if DefaultItemBufferSize < 1 {
		t.Errorf("DefaultItemBufferSize should be positive, got %d", DefaultItemBufferSize)
	}
	if DefaultIDBufferSize < 1 {
		t.Errorf("DefaultIDBufferSize should be positive, got %d", DefaultIDBufferSize)
	}
	if DefaultErrorBufferSize < 1 {
		t.Errorf("DefaultErrorBufferSize should be positive, got %d", DefaultErrorBufferSize)
	}

	// Verify defaults are consistent
	if DefaultItemBufferSize != 100 {
		t.Errorf("DefaultItemBufferSize changed from expected 100 to %d", DefaultItemBufferSize)
	}
	if DefaultIDBufferSize != 100 {
		t.Errorf("DefaultIDBufferSize changed from expected 100 to %d", DefaultIDBufferSize)
	}
	if DefaultErrorBufferSize != 100 {
		t.Errorf("DefaultErrorBufferSize changed from expected 100 to %d", DefaultErrorBufferSize)
	}
}
