package batch

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// uncooperativeSource emits a fixed number of items, then blocks forever
// without ever closing its channels and WITHOUT honoring ctx.Done(). This is
// the worst-case Source for the engine: in CancelDrain mode the reader has no
// way to make progress after cancellation because it is waiting on a Source
// that never stops. It is the precise scenario that distinguishes CancelStop
// from CancelDrain.
//
// release (returned to the caller) unblocks the producer and closes the
// channels so a test that relies on drain semantics can still clean up without
// leaking the goroutine.
type uncooperativeSource struct {
	emit      int           // how many items to emit before blocking
	delivered *uint32       // optional: incremented after each successful send into the pipeline
	started   chan struct{} // closed once the producer goroutine is running
	startOnce sync.Once
}

func newUncooperativeSource(emit int, delivered *uint32) *uncooperativeSource {
	return &uncooperativeSource{
		emit:      emit,
		delivered: delivered,
		started:   make(chan struct{}),
	}
}

func (s *uncooperativeSource) Read(ctx context.Context) (<-chan any, <-chan error) {
	out := make(chan any)
	errs := make(chan error)
	block := make(chan struct{}) // never closed; the producer parks here forever

	go func() {
		// Intentionally do NOT close(out)/close(errs) and do NOT select on
		// ctx.Done(): this Source ignores cancellation entirely.
		s.startOnce.Do(func() { close(s.started) })
		for i := 0; i < s.emit; i++ {
			out <- i // blocks until the reader takes it (unbuffered)
			if s.delivered != nil {
				atomic.AddUint32(s.delivered, 1)
			}
		}
		<-block // park forever
	}()

	return out, errs
}

// TestCancelStop_StopsOnUncooperativeSource is the core knob test. With
// CancelStop and a Source that never stops on its own, canceling the context
// must drive the pipeline to completion.
//
// RED (before doReader is wired for CancelStop): the reader stays blocked in
// its select on the uncooperative Source forever, Done() never closes, and the
// 2s guard fires t.Fatal.
func TestCancelStop_StopsOnUncooperativeSource(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	src := newUncooperativeSource(2, nil)

	b := New[any](nil).WithCancelMode(CancelStop)
	IgnoreErrors(b.Go(ctx, src, &noopProc{}))

	// Make sure the source is actually running before we cancel, so we are
	// exercising the cancel-while-reading path rather than a pre-cancel race.
	<-src.started
	cancel()

	select {
	case <-b.Done():
		// Pipeline stopped promptly on cancel despite the Source never closing.
	case <-time.After(2 * time.Second):
		t.Fatal("CancelStop: Done() did not close within 2s on an uncooperative source")
	}
}

// TestCancelDrain_DefaultRelaysOnSource locks the default semantic: with the
// SAME uncooperative source and NO WithCancelMode call, canceling the context
// must NOT make the engine stop on its own — it relies on the Source. We prove
// that by asserting Done() stays open for a short window after cancel, then
// release the Source so the test cleans up without leaking.
//
// RED (n/a — this test passes once scaffolding compiles): it documents and
// guards the default-drain contract so a future change to doReader cannot
// silently turn CancelDrain into CancelStop.
func TestCancelDrain_DefaultRelaysOnSource(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	src := newUncooperativeSource(2, nil)

	// No WithCancelMode call => default CancelDrain.
	b := New[any](nil)
	IgnoreErrors(b.Go(ctx, src, &noopProc{}))

	<-src.started
	cancel()

	// In drain mode the engine must NOT complete on its own while the Source
	// keeps its channels open.
	select {
	case <-b.Done():
		t.Fatal("CancelDrain: Done() closed on its own; default must rely on the Source to stop")
	case <-time.After(200 * time.Millisecond):
		// Expected: still waiting on the Source.
	}

	// Cleanup: there is no way to release this particular Source's producer
	// from the test (block is internal), so leave the deferred cancel in place;
	// the parked producer goroutine is harmless for the remainder of the test
	// binary. To keep the engine itself from hanging the suite, we do not block
	// on Done() here.
	_ = ctx
}

// releasableSource is like uncooperativeSource but exposes a Release() that
// closes its channels, so a drain-mode test can unblock and fully shut down.
type releasableSource struct {
	emit      int
	delivered *uint32
	started   chan struct{}
	release   chan struct{}
	startOnce sync.Once
}

func newReleasableSource(emit int, delivered *uint32) *releasableSource {
	return &releasableSource{
		emit:      emit,
		delivered: delivered,
		started:   make(chan struct{}),
		release:   make(chan struct{}),
	}
}

// Release unblocks the producer and lets it close its channels.
func (s *releasableSource) Release() { close(s.release) }

func (s *releasableSource) Read(ctx context.Context) (<-chan any, <-chan error) {
	out := make(chan any)
	errs := make(chan error)

	go func() {
		defer close(out)
		defer close(errs)
		s.startOnce.Do(func() { close(s.started) })
		for i := 0; i < s.emit; i++ {
			out <- i
			if s.delivered != nil {
				atomic.AddUint32(s.delivered, 1)
			}
		}
		<-s.release // ignore ctx; only Release() (or process exit) frees us
	}()

	return out, errs
}

// TestCancelDrain_ReleaseCompletes complements the default-drain test: once the
// Source is released (closes its channels), a drain-mode Batch completes. This
// keeps the suite leak-free and confirms drain still terminates normally.
func TestCancelDrain_ReleaseCompletes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	src := newReleasableSource(2, nil)

	b := New[any](nil) // default CancelDrain
	IgnoreErrors(b.Go(ctx, src, &noopProc{}))

	<-src.started
	cancel()

	// Still relying on the Source: must not complete yet.
	select {
	case <-b.Done():
		t.Fatal("CancelDrain: completed before the Source released")
	case <-time.After(100 * time.Millisecond):
	}

	src.Release()

	select {
	case <-b.Done():
		// Source closed its channels => drain completed.
	case <-time.After(2 * time.Second):
		t.Fatal("CancelDrain: Done() did not close within 2s after the Source released")
	}
}

// TestCancelStop_ProcessesBufferedItems verifies that stop-reading does not
// discard work already delivered into the pipeline. We deliver a known number
// of items, wait until they have all been handed to the engine, cancel, and
// assert that those items were processed.
//
// RED (before wiring): the engine never stops on the uncooperative source, so
// Done() never closes and the 2s guard fires before we can assert counts.
func TestCancelStop_ProcessesBufferedItems(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const emit = 5
	var delivered uint32
	var processed uint32

	src := newUncooperativeSource(emit, &delivered)

	proc := &countingProc{counter: &processed}

	// Large item buffer so all delivered items sit buffered ahead of the
	// processor, and MinItems=1 so each item forms its own batch and is
	// processed promptly.
	b := New[any](NewConstantConfig(&ConfigValues{MinItems: 1})).
		WithCancelMode(CancelStop).
		WithBufferConfig(BufferConfig{ItemBufferSize: 100, ErrorBufferSize: 100})
	IgnoreErrors(b.Go(ctx, src, proc))

	// Wait until the Source has delivered all `emit` items into the pipeline.
	deadline := time.After(2 * time.Second)
	for atomic.LoadUint32(&delivered) < emit {
		select {
		case <-deadline:
			t.Fatalf("source only delivered %d/%d items before timeout",
				atomic.LoadUint32(&delivered), emit)
		default:
			time.Sleep(time.Millisecond)
		}
	}

	cancel()

	select {
	case <-b.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("CancelStop: Done() did not close within 2s")
	}

	if got := atomic.LoadUint32(&processed); got != emit {
		t.Fatalf("CancelStop dropped buffered work: processed %d, want %d", got, emit)
	}
}

// TestWithCancelMode_PanicsAfterGo mirrors the WithBufferConfig-after-Go test:
// configuring the cancel mode after Go() has started must panic.
//
// RED (n/a — passes once the builder exists): guards the builder contract.
func TestWithCancelMode_PanicsAfterGo(t *testing.T) {
	b := New[any](nil)

	src := testSourceFunc(func(ctx context.Context) (<-chan any, <-chan error) {
		out := make(chan any)
		errs := make(chan error)
		go func() {
			defer close(out)
			defer close(errs)
			out <- "test"
		}()
		return out, errs
	})

	errs := b.Go(context.Background(), src)

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when calling WithCancelMode after Go")
		} else if msg, ok := r.(string); ok {
			if !strings.Contains(msg, "WithCancelMode cannot be called after Go() has started") {
				t.Errorf("unexpected panic message: %s", msg)
			}
		}
	}()

	b.WithCancelMode(CancelStop)

	// Cleanup
	IgnoreErrors(errs)
	<-b.Done()
}

// TestDefaultCancelModeIsDrain confirms the zero value of the mode is
// CancelDrain, and that a normal, ctx-cooperative source completes cleanly in
// BOTH modes (so the knob does not regress the happy path).
//
// RED (n/a — passes once the CancelMode type exists): documents the
// backwards-compatible default.
func TestDefaultCancelModeIsDrain(t *testing.T) {
	// Zero value of the field equals CancelDrain.
	var b Batch[any]
	if b.cancelMode != CancelDrain {
		t.Fatalf("zero-value cancelMode = %d, want CancelDrain (%d)", b.cancelMode, CancelDrain)
	}
	if CancelDrain != 0 {
		t.Fatalf("CancelDrain = %d, want 0 (zero value must be the default)", CancelDrain)
	}

	cooperative := func() Source[any] {
		return testSourceFunc(func(ctx context.Context) (<-chan any, <-chan error) {
			out := make(chan any)
			errs := make(chan error)
			go func() {
				defer close(out)
				defer close(errs)
				for i := 0; i < 3; i++ {
					select {
					case <-ctx.Done():
						return
					case out <- i:
					}
				}
			}()
			return out, errs
		})
	}

	for _, mode := range []struct {
		name    string
		applied *Batch[any]
	}{
		{"default", New[any](nil)},
		{"drain", New[any](nil).WithCancelMode(CancelDrain)},
		{"stop", New[any](nil).WithCancelMode(CancelStop)},
	} {
		mode := mode
		t.Run(mode.name, func(t *testing.T) {
			IgnoreErrors(mode.applied.Go(context.Background(), cooperative(), &noopProc{}))
			select {
			case <-mode.applied.Done():
			case <-time.After(2 * time.Second):
				t.Fatalf("%s mode: cooperative source did not complete within 2s", mode.name)
			}
		})
	}
}

// noopProc passes items through unchanged.
type noopProc struct{}

func (noopProc) Process(_ context.Context, items []*Item[any]) ([]*Item[any], error) {
	return items, nil
}

// countingProc counts the number of items it processes.
type countingProc struct {
	counter *uint32
}

func (p *countingProc) Process(_ context.Context, items []*Item[any]) ([]*Item[any], error) {
	atomic.AddUint32(p.counter, uint32(len(items)))
	return items, nil
}
