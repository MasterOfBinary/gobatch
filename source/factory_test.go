package source

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNewChannel(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		input := make(chan interface{}, 10)
		src, err := NewChannel(ChannelConfig{
			Input:      input,
			BufferSize: 50,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if src.Input != input {
			t.Error("input channel not set correctly")
		}

		if src.BufferSize != 50 {
			t.Errorf("expected BufferSize=50, got %d", src.BufferSize)
		}
	})

	t.Run("nil input channel", func(t *testing.T) {
		_, err := NewChannel(ChannelConfig{
			Input: nil,
		})

		if err == nil {
			t.Fatal("expected error for nil input channel")
		}

		if !strings.Contains(err.Error(), "input channel cannot be nil") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("zero buffer size", func(t *testing.T) {
		input := make(chan interface{})
		src, err := NewChannel(ChannelConfig{
			Input:      input,
			BufferSize: 0,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if src.BufferSize != 0 {
			t.Errorf("expected BufferSize=0, got %d", src.BufferSize)
		}

		// When used, it should fall back to defaultChannelBuffer
		ctx := context.Background()
		out, errs := src.Read(ctx)

		// Close input to trigger cleanup
		close(input)

		// Drain channels
		for range out {
		}
		for range errs {
		}
	})

	t.Run("negative buffer size", func(t *testing.T) {
		input := make(chan interface{})
		src, err := NewChannel(ChannelConfig{
			Input:      input,
			BufferSize: -1,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if src.BufferSize != -1 {
			t.Errorf("expected BufferSize=-1, got %d", src.BufferSize)
		}

		// Cleanup
		close(input)
	})
}

func TestNewError(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		errCh := make(chan error, 5)
		src, err := NewError(ErrorConfig{
			Errs:       errCh,
			BufferSize: 20,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if src.Errs != errCh {
			t.Error("error channel not set correctly")
		}

		if src.BufferSize != 20 {
			t.Errorf("expected BufferSize=20, got %d", src.BufferSize)
		}
	})

	t.Run("nil error channel", func(t *testing.T) {
		_, err := NewError(ErrorConfig{
			Errs: nil,
		})

		if err == nil {
			t.Fatal("expected error for nil error channel")
		}

		if !strings.Contains(err.Error(), "error channel cannot be nil") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("zero buffer size", func(t *testing.T) {
		errCh := make(chan error)
		src, err := NewError(ErrorConfig{
			Errs:       errCh,
			BufferSize: 0,
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if src.BufferSize != 0 {
			t.Errorf("expected BufferSize=0, got %d", src.BufferSize)
		}

		// Test that it works properly
		ctx := context.Background()
		out, errs := src.Read(ctx)

		// Send an error
		go func() {
			errCh <- errors.New("test error")
			close(errCh)
		}()

		// Should receive the error
		var gotError bool
		for err := range errs {
			if err.Error() == "test error" {
				gotError = true
			}
		}

		if !gotError {
			t.Error("did not receive expected error")
		}

		// Output channel should be closed
		for range out {
			t.Error("unexpected output from error source")
		}
	})
}

func TestNewNil(t *testing.T) {
	src := NewNil()

	if src == nil {
		t.Fatal("NewNil returned nil")
	}

	// Test that it behaves correctly
	ctx := context.Background()
	out, errs := src.Read(ctx)

	// Both channels should close immediately
	for range out {
		t.Error("unexpected output from nil source")
	}

	for range errs {
		t.Error("unexpected error from nil source")
	}
}
