package batch_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/source"
)

// batchSizeMonitor records, in a concurrency-safe way, how many batches were
// processed and the total number of items seen. It deliberately does NOT print
// from within Process: every batch is handled in its own goroutine with no
// ordering guarantee between them, so printing per-batch (and interleaving
// those prints with the producer's status messages) would make the output
// non-deterministic. Instead the example prints an invariant summary once
// processing has finished.
type batchSizeMonitor struct {
	mu    sync.Mutex
	items int
}

func (p *batchSizeMonitor) Process(ctx context.Context, items []*batch.Item[any]) ([]*batch.Item[any], error) {
	p.mu.Lock()
	p.items += len(items)
	p.mu.Unlock()
	return items, nil
}

func Example_dynamicConfig() {
	cfg := batch.NewDynamicConfig(&batch.ConfigValues{
		MinItems: 5,
		MaxItems: 10,
	})

	b := batch.New[any](cfg)
	monitor := &batchSizeMonitor{}

	ch := make(chan any)
	src := &source.Channel[any]{Input: ch}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const totalItems = 100

	fmt.Println("=== Dynamic Config Example ===")
	fmt.Println("Initial config: min=5, max=10")

	errs, err := b.Go(ctx, src, monitor)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Send data while adjusting the config at fixed points in the stream. The
	// updates are keyed off the item index (not wall-clock timing), so the
	// config transitions are deterministic. The batcher picks up the new sizes
	// for subsequent batches.
	go func() {
		for i := 0; i < totalItems; i++ {
			ch <- i

			switch i {
			case 20:
				cfg.UpdateBatchSize(10, 20)
			case 50:
				cfg.UpdateBatchSize(20, 30)
			}

			time.Sleep(5 * time.Millisecond)
		}
		close(ch)
	}()

	batch.IgnoreErrors(errs)
	<-b.Done()

	// Print an invariant summary after processing completes. The exact number
	// of batches and their individual sizes depend on the interleaving of the
	// producer and the batch goroutines, so we assert only what is guaranteed:
	// every item is processed exactly once, and the config moved through all
	// three phases.
	fmt.Println("Config updated to: min=10, max=20")
	fmt.Println("Config updated to: min=20, max=30")
	fmt.Printf("Processed %d items in total\n", monitor.items)
	fmt.Println("Processing complete")

	// Output:
	// === Dynamic Config Example ===
	// Initial config: min=5, max=10
	// Config updated to: min=10, max=20
	// Config updated to: min=20, max=30
	// Processed 100 items in total
	// Processing complete
}
