package batch_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	. "github.com/MasterOfBinary/gobatch/batch"
)

// manyErrorsSource emits N errors (and no data items). It is used to fill the
// error buffer so the pipeline wedges when nobody drains b.errs.
type manyErrorsSource struct {
	N int
}

func (s *manyErrorsSource) Read(ctx context.Context) (<-chan any, <-chan error) {
	out := make(chan any)
	errs := make(chan error)
	go func() {
		defer close(out)
		defer close(errs)
		for i := 0; i < s.N; i++ {
			select {
			case <-ctx.Done():
				return
			case errs <- fmt.Errorf("err %d", i):
			}
		}
	}()
	return out, errs
}

// TestCancelUnblocksWedgedReader verifies that cancelling the context unblocks a
// pipeline that has wedged because the error buffer filled and nobody is
// draining it. Before the fix the internal sends to b.errs were unguarded
// blocking sends, so cancel() did not free the reader and Done() never closed.
//
// The test is bounded by a timeout so the RED state fails fast instead of
// hanging the suite.
func TestCancelUnblocksWedgedReader(t *testing.T) {
	// Tiny error buffer so it fills almost immediately, and a source that emits
	// far more errors than the buffer can hold.
	b := New[any](NewConstantConfig(&ConfigValues{})).
		WithBufferConfig(BufferConfig{ErrorBufferSize: 1})

	src := &manyErrorsSource{N: 1000}

	ctx, cancel := context.WithCancel(context.Background())

	// Start processing but deliberately never drain the returned error channel.
	_ = b.Go(ctx, src)

	// Give the reader time to fill the 1-slot error buffer and wedge.
	time.Sleep(50 * time.Millisecond)

	// Cancelling must break the pipeline out of the blocked send.
	cancel()

	select {
	case <-b.Done():
		// Pipeline completed after cancellation - correct.
	case <-time.After(2 * time.Second):
		t.Fatal("deadlock: Done() did not close within 2s after cancel; " +
			"the wedged reader was not unblocked by context cancellation")
	}
}

// TestCancelUnblocksWedgedProcessor verifies the same guarantee for the
// per-batch processor goroutine: if a processor returns errors that cannot be
// delivered because the error buffer is full and nobody is draining, cancelling
// the context must let the goroutine exit so the pipeline can complete.
func TestCancelUnblocksWedgedProcessor(t *testing.T) {
	b := New[any](NewConstantConfig(&ConfigValues{MinItems: 1})).
		WithBufferConfig(BufferConfig{ErrorBufferSize: 1})

	// A source that emits many data items so many batches are produced, each of
	// which fails and tries to send a ProcessorError.
	items := make([]any, 1000)
	for i := range items {
		items[i] = i
	}
	src := &sliceSource{items: items}

	procErr := errors.New("always fails")
	proc := &alwaysErrProcessor{err: procErr}

	ctx, cancel := context.WithCancel(context.Background())

	_ = b.Go(ctx, src, proc)

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-b.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("deadlock: Done() did not close within 2s after cancel; " +
			"the wedged processor goroutine was not unblocked by context cancellation")
	}
}

// alwaysErrProcessor returns a processor-wide error for every batch.
type alwaysErrProcessor struct {
	err error
}

func (p *alwaysErrProcessor) Process(ctx context.Context, items []*Item[any]) ([]*Item[any], error) {
	return items, p.err
}
