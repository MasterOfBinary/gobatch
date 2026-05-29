package batch_test

import (
	"context"
	"sync"
	"testing"

	. "github.com/MasterOfBinary/gobatch/batch"
)

// TestReuse_AfterCollectErrors verifies the CollectErrors completion
// contract: once CollectErrors returns, the Batch is fully torn down
// (Done closed, running cleared) so the same Batch can be reused
// immediately without hitting the "Concurrent calls" panic.
//
// This is a regression test for shutdown ordering in doProcessors: if
// b.errs is closed before b.done/b.running are finalized, a caller that
// follows the documented "no need to wait on Done()" guidance and reuses
// the Batch can intermittently panic. Run with -race to surface it.
func TestReuse_AfterCollectErrors(t *testing.T) {
	b := New[any](NewConstantConfig(&ConfigValues{MinItems: 1, MaxItems: 5}))

	for i := 0; i < 200; i++ {
		src := &testSource{Items: []any{1, 2, 3, 4, 5}}
		proc := &countProcessor{}

		// CollectErrors blocks until b.errs is closed; per its docs the
		// caller need not also wait on Done(). Reuse b on the very next
		// line — this panics if running is still true at that point.
		errs := CollectErrors(b.Go(context.Background(), src, proc))
		for range errs {
		}
	}
}

// TestReuse_AfterDone verifies the other synchronization path flagged in
// review: a caller that waits on <-Done() and then immediately reuses the
// Batch must not hit the "Concurrent calls" panic. This requires that
// running is cleared no later than Done() becoming observable. Shutdown
// closes done and clears running inside the same b.mu section that
// doProcessors holds across both, so a Go() from a Done() waiter blocks
// on the mutex until running is false. Run with -race -count to surface
// any regression.
func TestReuse_AfterDone(t *testing.T) {
	b := New[any](NewConstantConfig(&ConfigValues{MinItems: 1, MaxItems: 5}))

	for i := 0; i < 200; i++ {
		src := &testSource{Items: []any{1, 2, 3, 4, 5}}
		proc := &countProcessor{}

		errs := b.Go(context.Background(), src, proc)
		IgnoreErrors(errs)

		// Wake on Done() and reuse the Batch immediately. If running is
		// still true when Done() is observable, the next Go() panics.
		<-b.Done()
	}
}

// idRecorder is a processor that records the ID of every item it processes.
type idRecorder struct {
	mu  sync.Mutex
	ids []uint64
}

func (r *idRecorder) Process(_ context.Context, items []*Item[any]) ([]*Item[any], error) {
	r.mu.Lock()
	for _, it := range items {
		r.ids = append(r.ids, it.ID)
	}
	r.mu.Unlock()
	return items, nil
}

// TestReuse_RestartsIDsFromZero pins the core ID contract: every run of a
// reused Batch numbers items 0..n-1. This guards against the per-run reset
// silently regressing (e.g. IDs starting at 1, or not resetting between runs)
// — a failure mode the existing reuse tests, which only assert "no panic",
// would not catch.
func TestReuse_RestartsIDsFromZero(t *testing.T) {
	// MinItems == MaxItems == 5 with a 5-item source yields exactly one batch
	// per run, so the recorded IDs are 0..4 in order.
	b := New[any](NewConstantConfig(&ConfigValues{MinItems: 5, MaxItems: 5}))

	for iter := 0; iter < 5; iter++ {
		rec := &idRecorder{}
		IgnoreErrors(b.Go(context.Background(), &testSource{Items: []any{10, 20, 30, 40, 50}}, rec))
		<-b.Done()

		want := []uint64{0, 1, 2, 3, 4}
		if len(rec.ids) != len(want) {
			t.Fatalf("iteration %d: got %d IDs %v, want %v", iter, len(rec.ids), rec.ids, want)
		}
		for i, id := range rec.ids {
			if id != want[i] {
				t.Fatalf("iteration %d: IDs = %v, want %v", iter, rec.ids, want)
			}
		}
	}
}
