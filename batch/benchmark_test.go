package batch_test

import (
	"context"
	"testing"

	. "github.com/MasterOfBinary/gobatch/batch"
)

// benchSource emits the integers [0, n) as fast as it can, respecting context
// cancellation. It exists to drive a high item rate through the pipeline so the
// per-item ID-assignment path in doReader is exercised.
type benchSource struct{ n int }

func (s *benchSource) Read(ctx context.Context) (<-chan any, <-chan error) {
	out := make(chan any)
	errs := make(chan error)
	go func() {
		defer close(out)
		defer close(errs)
		for i := 0; i < s.n; i++ {
			select {
			case <-ctx.Done():
				return
			case out <- i:
			}
		}
	}()
	return out, errs
}

// benchPassthrough is a no-op processor: it returns the batch unchanged so the
// benchmark measures pipeline/ID overhead rather than processing work.
type benchPassthrough struct{}

func (benchPassthrough) Process(_ context.Context, items []*Item[any]) ([]*Item[any], error) {
	return items, nil
}

// BenchmarkBatchThroughput measures end-to-end throughput for a fixed number of
// items per run. Every item gets an ID assigned in doReader, so this captures
// the cost of the ID-generation mechanism (the inline counter on this branch
// vs. the dedicated goroutine + buffered channel on master) plus the per-Go
// setup cost. A fresh Batch is created each iteration, which is also the
// required usage: a Batch is single-use.
//
// Compare across branches with:
//
//	go test -run '^$' -bench BenchmarkBatchThroughput -benchmem -count=10 ./batch
//
// then feed both outputs to benchstat.
func BenchmarkBatchThroughput(b *testing.B) {
	const itemsPerRun = 1000
	cfg := NewConstantConfig(&ConfigValues{MaxItems: 100})
	proc := benchPassthrough{}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bt := New[any](cfg)
		errs, err := bt.Go(ctx, &benchSource{n: itemsPerRun}, proc)
		if err != nil {
			b.Fatal(err)
		}
		IgnoreErrors(errs)
		<-bt.Done()
	}
}
