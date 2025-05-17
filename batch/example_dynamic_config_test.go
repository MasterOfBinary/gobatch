package batch_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/source"
)

type batchSizeMonitor struct {
	mu      sync.Mutex
	name    string
	batches int
	items   int
}

func (p *batchSizeMonitor) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	p.mu.Lock()
	p.batches++
	p.items += len(items)
	batchSize := len(items)
	name := p.name
	p.mu.Unlock()

	fmt.Printf("[%s] Batch size: %d\n", name, batchSize)
	return items, nil
}

func Example_dynamicConfig() {
	cfg := batch.NewDynamicConfig(&batch.ConfigValues{
		MinItems: 5,
		MaxItems: 10,
	})

	b := batch.New(cfg)
	monitor := &batchSizeMonitor{name: "Dynamic"}

	ch := make(chan interface{})
	src := &source.Channel{Input: ch}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Println("=== Dynamic Config Example ===")

	errs := b.Go(ctx, src, monitor)

	// Simulate sending data and changing config dynamically
	go func() {
		for i := 0; i < 100; i++ {
			ch <- i

                       switch i {
                       case 20:
                               fmt.Println("*** Updating batch size: min=10, max=20 ***")
                               cfg.UpdateBatchSize(10, 20)
                       case 50:
                               fmt.Println("*** Updating batch size: min=20, max=30 ***")
                               cfg.UpdateBatchSize(20, 30)
                       }

			time.Sleep(5 * time.Millisecond)
		}
		close(ch)
	}()

	batch.IgnoreErrors(errs)
	<-b.Done()

	fmt.Println("Processing complete")

	// Output:
	// === Dynamic Config Example ===
	// [Dynamic] Batch size: 5
	// [Dynamic] Batch size: 5
	// [Dynamic] Batch size: 5
	// [Dynamic] Batch size: 5
	// *** Updating batch size: min=10, max=20 ***
	// [Dynamic] Batch size: 5
	// [Dynamic] Batch size: 10
	// [Dynamic] Batch size: 10
	// *** Updating batch size: min=20, max=30 ***
	// [Dynamic] Batch size: 10
	// [Dynamic] Batch size: 20
	// [Dynamic] Batch size: 20
	// [Dynamic] Batch size: 5
	// Processing complete
}
