package batch_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
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

// panicProcessor panics inside Process, simulating a buggy user processor.
type panicProcessor struct {
	msg string
}

func (p *panicProcessor) Process(ctx context.Context, items []*Item[any]) ([]*Item[any], error) {
	panic(p.msg)
}

// TestPanicInProcessorIsRecovered verifies that a panic in a user Processor does
// not crash the host process. Before the fix, the per-batch goroutine called
// proc.Process with no recover, so a panic took down the whole process. After
// the fix the panic is recovered, surfaced as a ProcessorError on the error
// channel, and the pipeline completes cleanly (errs and done close).
func TestPanicInProcessorIsRecovered(t *testing.T) {
	b := New[any](NewConstantConfig(&ConfigValues{MinItems: 1}))
	src := &testSource{Items: []any{1, 2, 3}}
	proc := &panicProcessor{msg: "boom in processor"}

	errs := b.Go(context.Background(), src, proc)

	var sawPanicErr bool
	for err := range errs {
		var pe *ProcessorError
		if errors.As(err, &pe) {
			// The recovered panic must be surfaced and carry the panic value.
			if containsStr(err.Error(), "processor panic") &&
				containsStr(err.Error(), "boom in processor") {
				sawPanicErr = true
			}
		}
	}

	// The pipeline must complete rather than crash or hang.
	select {
	case <-b.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done() did not close within 2s after a panicking processor")
	}

	if !sawPanicErr {
		t.Fatal("expected a ProcessorError describing the recovered panic")
	}
}

// TestMaxTimeMultipleIdleCyclesThenLateItem exercises several idle MaxTime
// cycles (the batch stays empty past MaxTime more than once) and then delivers
// a late item, asserting it is still processed. This locks in the re-arm
// behavior so the timer refactor (single *time.Timer with Reset instead of a
// fresh time.After each idle fire) cannot regress it.
func TestMaxTimeMultipleIdleCyclesThenLateItem(t *testing.T) {
	cfg := NewConstantConfig(&ConfigValues{
		MinItems: 10,                    // high, so MinItems alone never triggers
		MaxTime:  50 * time.Millisecond, // fires repeatedly while idle
	})

	b := New[any](cfg)

	input := make(chan any)
	src := &chanSource{in: input}

	var processed int32
	proc := &countingProc{n: &processed}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errs := b.Go(ctx, src, proc)
	go func() {
		for range errs {
		}
	}()

	// Let several idle MaxTime cycles elapse (4 x 50ms = 200ms+).
	time.Sleep(220 * time.Millisecond)

	// Now deliver a late item; it must still be picked up and processed.
	input <- 42

	// Give the late item time to be batched (within one MaxTime cycle) and run.
	deadline := time.After(2 * time.Second)
	for atomic.LoadInt32(&processed) == 0 {
		select {
		case <-deadline:
			t.Fatal("late item was not processed after multiple idle MaxTime cycles")
		case <-time.After(10 * time.Millisecond):
		}
	}

	close(input)
	select {
	case <-b.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done() did not close within 2s")
	}

	if got := atomic.LoadInt32(&processed); got != 1 {
		t.Errorf("expected exactly 1 item processed, got %d", got)
	}
}

// chanSource adapts a caller-owned input channel into a Source, respecting
// context cancellation.
type chanSource struct {
	in chan any
}

func (s *chanSource) Read(ctx context.Context) (<-chan any, <-chan error) {
	out := make(chan any)
	errs := make(chan error)
	go func() {
		defer close(out)
		defer close(errs)
		for {
			select {
			case <-ctx.Done():
				return
			case v, ok := <-s.in:
				if !ok {
					return
				}
				select {
				case <-ctx.Done():
					return
				case out <- v:
				}
			}
		}
	}()
	return out, errs
}

// countingProc atomically counts the items it processes.
type countingProc struct {
	n *int32
}

func (p *countingProc) Process(ctx context.Context, items []*Item[any]) ([]*Item[any], error) {
	atomic.AddInt32(p.n, int32(len(items)))
	return items, nil
}

// TestDoneRace verifies there is no data race between Done() reading b.done and
// Go() writing it. Done() previously read b.done without holding b.mu, while Go
// writes it under the lock. Run with -race to surface the report (RED) before
// the fix; after the fix it must be clean.
func TestDoneRace(t *testing.T) {
	b := New[any](NewConstantConfig(&ConfigValues{}))
	src := &testSource{Items: []any{1, 2, 3}}

	stop := make(chan struct{})
	done := make(chan struct{})

	// Hammer Done() concurrently with Go().
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				_ = b.Done()
			}
		}
	}()

	// Let the reader goroutine spin up and start calling Done().
	time.Sleep(5 * time.Millisecond)

	errs := b.Go(context.Background(), src)
	go func() {
		for range errs {
		}
	}()

	<-b.Done()
	close(stop)
	<-done
}

// containsStr is a tiny substring helper to avoid importing strings here.
func containsStr(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
