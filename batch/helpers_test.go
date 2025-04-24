package batch

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestIgnoreErrors(t *testing.T) {
	done := make(chan struct{})
	errs := make(chan error)

	go func() {
		for i := 0; i < 10; i++ {
			errs <- errors.New("error " + strconv.Itoa(i))
		}
		close(errs)
		close(done)
	}()

	IgnoreErrors(errs)

	// Make sure the first goroutine is able to complete. Otherwise it
	// wasn't able to write to the error channel
	select {
	case <-done:
		break
	case <-time.After(time.Second):
		t.Error("Writing goroutine didn't complete")
	}
}

func TestCollectErrors(t *testing.T) {
	t.Run("collect all errors", func(t *testing.T) {
		// Create error channel and send some errors
		errs := make(chan error, 3)
		expectedErrors := []error{
			errors.New("error 1"),
			errors.New("error 2"),
			errors.New("error 3"),
		}

		for _, err := range expectedErrors {
			errs <- err
		}
		close(errs)

		// Collect errors
		collectedErrors := CollectErrors(errs)

		// Verify we got the expected errors
		if len(collectedErrors) != len(expectedErrors) {
			t.Errorf("expected %d errors, got %d", len(expectedErrors), len(collectedErrors))
		}

		// Check the error messages (since we can't compare error instances directly)
		for i, err := range collectedErrors {
			if err.Error() != expectedErrors[i].Error() {
				t.Errorf("expected error %q, got %q", expectedErrors[i].Error(), err.Error())
			}
		}
	})

	t.Run("nil error channel", func(t *testing.T) {
		// Verify nil channel is handled correctly
		result := CollectErrors(nil)
		if result != nil {
			t.Errorf("expected nil result for nil channel, got %v", result)
		}
	})
}

func TestRunBatchAndWait(t *testing.T) {
	// Create test data
	batch := New(NewConstantConfig(&ConfigValues{}))
	src := &testSource{
		Items:   []interface{}{1, 2, 3},
		WithErr: errors.New("source error"),
	}

	var count uint32
	proc := &countProcessor{count: &count}

	// Run the batch and collect errors
	errors := RunBatchAndWait(context.Background(), batch, src, proc)

	// Verify processing occurred
	if atomic.LoadUint32(&count) != 3 {
		t.Errorf("expected 3 items processed, got %d", count)
	}

	// Verify error was collected
	if len(errors) == 0 {
		t.Error("expected at least one error, got none")
	}
}

func TestExecuteBatches(t *testing.T) {
	// Create multiple batches with sources and processors
	batch1 := New(NewConstantConfig(&ConfigValues{}))
	src1 := &testSource{Items: []interface{}{1, 2, 3}}
	var count1 uint32
	proc1 := &countProcessor{count: &count1}

	batch2 := New(NewConstantConfig(&ConfigValues{}))
	src2 := &testSource{
		Items:   []interface{}{4, 5},
		WithErr: errors.New("source 2 error"),
	}
	var count2 uint32
	proc2 := &countProcessor{count: &count2}

	// Configure the batches
	configs := []*BatchConfig{
		{B: batch1, S: src1, P: []Processor{proc1}},
		{B: batch2, S: src2, P: []Processor{proc2}},
	}

	// Execute all batches
	errors := ExecuteBatches(context.Background(), configs...)

	// Verify processing occurred for all batches
	if atomic.LoadUint32(&count1) != 3 {
		t.Errorf("batch1: expected 3 items processed, got %d", count1)
	}

	if atomic.LoadUint32(&count2) != 2 {
		t.Errorf("batch2: expected 2 items processed, got %d", count2)
	}

	// Verify we got the expected error
	if len(errors) == 0 {
		t.Error("expected at least one error, got none")
	}
}

// Helper types for testing

type countProcessor struct {
	count *uint32
}

func (p *countProcessor) Process(ctx context.Context, items []*Item) ([]*Item, error) {
	atomic.AddUint32(p.count, uint32(len(items)))
	return items, nil
}

type testSource struct {
	Items   []interface{}
	WithErr error
}

func (s *testSource) Read(ctx context.Context) (<-chan interface{}, <-chan error) {
	out := make(chan interface{})
	errs := make(chan error, 1)
	go func() {
		defer close(out)
		defer close(errs)
		for _, item := range s.Items {
			select {
			case <-ctx.Done():
				return
			case out <- item:
			}
		}
		if s.WithErr != nil {
			errs <- s.WithErr
		}
	}()
	return out, errs
}
