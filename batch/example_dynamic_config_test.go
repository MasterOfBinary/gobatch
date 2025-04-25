package batch_test

import (
	"context"
	"fmt"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/source"
)

// batchSizeMonitor is a simple processor that logs batch sizes when they're processed.
// It demonstrates how to track batch processing metrics without modifying the items.
type batchSizeMonitor struct {
	name string
	// Track processing statistics
	processedBatches int
	totalItems       int
}

// Process implements the Processor interface by logging batch information
// and tracking basic processing statistics. It returns the items unmodified.
func (p *batchSizeMonitor) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	p.processedBatches++
	p.totalItems += len(items)
	fmt.Printf("[%s] Processing batch of %d items\n", p.name, len(items))
	return items, nil
}

// Example_dynamicConfig demonstrates how to create and use a Config
// implementation that allows real-time configuration changes during batch processing.
//
// Key concepts demonstrated:
// 1. Creating a thread-safe configuration that can change during processing
// 2. Updating batch parameters in response to simulated system conditions
// 3. How configuration changes affect subsequent batch sizes
// 4. Proper propagation of configuration changes to the batch processor
//
// This pattern is useful for systems that need to adapt to changing workloads,
// such as:
// - Reducing batch sizes during high system load
// - Increasing batch sizes during low utilization periods
// - Adjusting timing constraints based on downstream system capacity
// - Implementing backpressure mechanisms
func Example_dynamicConfig() {
	// Create an adaptive configuration that adjusts batch size dynamically
	cfg := batch.NewDynamicConfig(&batch.ConfigValues{
		MinItems: 5,
		MaxItems: 10,
		MinTime:  0, // no time constraints initially
		MaxTime:  0, // no time constraints initially
	})

	// Create batch processor with our adaptive config
	b := batch.New(cfg)
	monitor := &batchSizeMonitor{name: "Dynamic Processor"}

	// Create a channel source with 100 integers
	ch := make(chan interface{})
	s := &source.Channel{
		Input: ch,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start processing
	fmt.Println("Starting processing with dynamic configuration...")
	errs := b.Go(ctx, s, monitor)

	// Feed data while changing configuration during processing
	go func() {
		for i := 0; i < 100; i++ {
			ch <- i

			// Adjust configuration at specific points to simulate
			// dynamic workload-based configuration changes
			if i == 20 {
				// Simulate increased system load - reduce batch size
				fmt.Println("*** Configuration updated: min=10, max=20 ***")
				cfg.UpdateBatchSize(10, 20)
			} else if i == 50 {
				// Simulate heavy system load - further increase batch parameters
				fmt.Println("*** Configuration updated: min=20, max=30 ***")
				cfg.UpdateBatchSize(20, 30)

				// Note: We could also adjust timing parameters with UpdateTiming()
				// but this is omitted to keep the test output deterministic
				// cfg.UpdateTiming(50*time.Millisecond, 200*time.Millisecond)
			}

			time.Sleep(time.Millisecond * 5)
		}
		close(ch)
	}()

	// Wait for completion
	batch.IgnoreErrors(errs)
	<-b.Done()
	fmt.Println("Processing complete")

	// Output:
	// Starting processing with dynamic configuration...
	// [Dynamic Processor] Processing batch of 5 items
	// [Dynamic Processor] Processing batch of 5 items
	// [Dynamic Processor] Processing batch of 5 items
	// [Dynamic Processor] Processing batch of 5 items
	// *** Configuration updated: min=10, max=20 ***
	// [Dynamic Processor] Processing batch of 5 items
	// [Dynamic Processor] Processing batch of 10 items
	// [Dynamic Processor] Processing batch of 10 items
	// *** Configuration updated: min=20, max=30 ***
	// [Dynamic Processor] Processing batch of 10 items
	// [Dynamic Processor] Processing batch of 20 items
	// [Dynamic Processor] Processing batch of 20 items
	// [Dynamic Processor] Processing batch of 5 items
	// Processing complete
}
