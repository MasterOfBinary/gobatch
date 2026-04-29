package batch_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	. "github.com/MasterOfBinary/gobatch/batch"
)

// fnSource implements Source[any] by calling a function.
type fnSource struct {
	fn func(ctx context.Context) (<-chan any, <-chan error)
}

func (s *fnSource) Read(ctx context.Context) (<-chan any, <-chan error) {
	return s.fn(ctx)
}

// TestCancellation_BufferedItemsDrained verifies that items already read from
// the source and sitting in the internal buffer are still processed after
// context cancellation, honoring Go()'s contract that already-read items are
// never dropped.
//
// The source pushes every item synchronously and then blocks on ctx.Done,
// so it cannot lose items in response to cancel. MinItems is set above the
// total so waitForItems can never satisfy its threshold — only the new
// ctxDone branch can deliver the accumulated batch.
func TestCancellation_BufferedItemsDrained(t *testing.T) {
	const totalItems = 20

	// delivered is closed by the source goroutine immediately after every
	// item has been received by doReader. Because `out` is unbuffered, each
	// `out <- i` only returns once doReader has taken the item, so closing
	// `delivered` after the loop guarantees all totalItems items are in the
	// pipeline (in b.items or in transit inside doReader) — no wall-clock
	// sleep required.
	delivered := make(chan struct{})

	src := &fnSource{fn: func(ctx context.Context) (<-chan any, <-chan error) {
		out := make(chan any)
		errs := make(chan error)
		go func() {
			defer close(out)
			defer close(errs)
			for i := 0; i < totalItems; i++ {
				out <- i
			}
			close(delivered)
			<-ctx.Done()
		}()
		return out, errs
	}}

	var (
		mu           sync.Mutex
		processedIDs = make(map[uint64]bool)
	)

	proc := &testProcessor{
		processFn: func(_ context.Context, items []*Item[any]) ([]*Item[any], error) {
			mu.Lock()
			for _, item := range items {
				processedIDs[item.ID] = true
			}
			mu.Unlock()
			return items, nil
		},
	}

	config := NewConstantConfig(&ConfigValues{
		MinItems: totalItems + 5,
		MaxTime:  time.Hour,
	})

	b := New[any](config)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := b.Go(ctx, src, proc)

	// Wait until every item has been handed off to doReader, then cancel.
	<-delivered
	cancel()

	for range errCh {
	}
	<-b.Done()

	mu.Lock()
	count := len(processedIDs)
	mu.Unlock()

	if count != totalItems {
		t.Fatalf("expected all %d buffered items to be processed after cancel, got %d", totalItems, count)
	}
	for i := uint64(0); i < totalItems; i++ {
		if !processedIDs[i] {
			t.Errorf("item ID %d was buffered but not processed (dropped)", i)
		}
	}
}

// TestCancellation_SourceErrorNoRace verifies that a source emitting an error
// at cancellation time does not race with doProcessors closing the error
// channel. This test should be run with -race.
func TestCancellation_SourceErrorNoRace(t *testing.T) {
	// Run many iterations to maximize chance of exposing a race
	for i := 0; i < 50; i++ {
		src := &fnSource{fn: func(ctx context.Context) (<-chan any, <-chan error) {
			out := make(chan any)
			errs := make(chan error, 1)
			go func() {
				defer close(out)
				defer close(errs)

				for j := 0; j < 5; j++ {
					select {
					case out <- j:
					case <-ctx.Done():
						errs <- fmt.Errorf("source cancelled: %w", ctx.Err())
						return
					}
					time.Sleep(5 * time.Millisecond)
				}

				<-ctx.Done()
				errs <- fmt.Errorf("source cancelled: %w", ctx.Err())
			}()
			return out, errs
		}}

		proc := &testProcessor{
			processFn: func(ctx context.Context, items []*Item[any]) ([]*Item[any], error) {
				return items, nil
			},
		}

		b := New[any](NewConstantConfig(&ConfigValues{
			MinItems: 2,
			MaxTime:  100 * time.Millisecond,
		}))

		ctx, cancel := context.WithCancel(context.Background())

		errCh := b.Go(ctx, src, proc)

		time.Sleep(15 * time.Millisecond)
		cancel()

		// Drain errors — if there's a send-on-closed-channel this panics
		for range errCh {
		}
		<-b.Done()
	}
}

// TestCancellation_ItemErrorType verifies that per-item errors are reported as
// ItemError (not ProcessorError) and include the correct item ID.
func TestCancellation_ItemErrorType(t *testing.T) {
	src := &testSource{Items: []any{1, 2, 3, 4, 5}}

	proc := &testProcessor{
		processFn: func(ctx context.Context, items []*Item[any]) ([]*Item[any], error) {
			for _, item := range items {
				item.Error = fmt.Errorf("bad item %d", item.Data)
			}
			return items, nil
		},
	}

	b := New[any](NewConstantConfig(&ConfigValues{}))
	errCh := b.Go(context.Background(), src, proc)

	var itemErrors []*ItemError
	for err := range errCh {
		var ie *ItemError
		if errors.As(err, &ie) {
			itemErrors = append(itemErrors, ie)
		} else {
			t.Errorf("expected ItemError, got %T: %v", err, err)
		}
	}
	<-b.Done()

	if len(itemErrors) != 5 {
		t.Fatalf("expected 5 ItemErrors, got %d", len(itemErrors))
	}

	// Per-item batches dispatch on separate goroutines, so error arrival
	// order is non-deterministic. Sort by ItemID before checking the set.
	sort.Slice(itemErrors, func(i, j int) bool {
		return itemErrors[i].ItemID < itemErrors[j].ItemID
	})
	for i, ie := range itemErrors {
		if ie.ItemID != uint64(i) {
			t.Errorf("position %d: expected ItemID=%d, got %d", i, i, ie.ItemID)
		}
	}
}

// TestCancellation_ProcessorErrorDistinct verifies that processor-wide errors
// are still reported as ProcessorError, distinct from ItemError.
func TestCancellation_ProcessorErrorDistinct(t *testing.T) {
	src := &testSource{Items: []any{1, 2, 3}}

	procErr := errors.New("processor failed")
	proc := &testProcessor{
		processFn: func(ctx context.Context, items []*Item[any]) ([]*Item[any], error) {
			items[0].Error = errors.New("item bad")
			return items, procErr
		},
	}

	b := New[any](NewConstantConfig(&ConfigValues{MinItems: 3}))
	errCh := b.Go(context.Background(), src, proc)

	var (
		procErrors int
		itemErrs   int
	)
	for err := range errCh {
		var pe *ProcessorError
		var ie *ItemError
		switch {
		case errors.As(err, &pe):
			procErrors++
			if !errors.Is(err, procErr) {
				t.Errorf("ProcessorError should wrap original: got %v", err)
			}
		case errors.As(err, &ie):
			itemErrs++
		default:
			t.Errorf("unexpected error type %T: %v", err, err)
		}
	}
	<-b.Done()

	if procErrors == 0 {
		t.Error("expected at least one ProcessorError")
	}
	if itemErrs == 0 {
		t.Error("expected at least one ItemError")
	}
}
