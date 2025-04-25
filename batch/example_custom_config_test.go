package batch_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
)

// sliceSource implements batch.Source by reading items from a slice.
// It demonstrates a simple source implementation for testing and examples.
type sliceSource struct {
	items []interface{}
	delay time.Duration
}

// Read implements batch.Source.Read by streaming items from the internal slice.
// It respects context cancellation and properly closes channels when finished.
func (s *sliceSource) Read(ctx context.Context) (<-chan interface{}, <-chan error) {
	out := make(chan interface{}, 100)
	errs := make(chan error)

	go func() {
		defer close(out)
		defer close(errs)

		for _, item := range s.items {
			if s.delay > 0 {
				time.Sleep(s.delay)
			}

			select {
			case <-ctx.Done():
				return
			case out <- item:
				// Item sent successfully
			}
		}
	}()

	return out, errs
}

// loadBasedConfig implements batch.Config with parameters that dynamically
// adjust based on simulated system load. This demonstrates how to create
// custom configurations that respond to external conditions.
type loadBasedConfig struct {
	mu           sync.RWMutex
	currentLoad  int // 0-100 representing system load
	baseMinItems uint64
	baseMaxItems uint64
	baseMinTime  time.Duration
	baseMaxTime  time.Duration
}

// newLoadBasedConfig creates a new load-based configuration with the specified base parameters.
// The actual configuration values returned by Get() will be dynamically adjusted
// based on the current system load.
//
// Parameters:
//   - baseMin: Base minimum items per batch
//   - baseMax: Base maximum items per batch
//   - minTime: Base minimum time between batches
//   - maxTime: Base maximum time between batches
func newLoadBasedConfig(baseMin, baseMax uint64, minTime, maxTime time.Duration) *loadBasedConfig {
	return &loadBasedConfig{
		currentLoad:  50, // Start with medium load
		baseMinItems: baseMin,
		baseMaxItems: baseMax,
		baseMinTime:  minTime,
		baseMaxTime:  maxTime,
	}
}

// Get implements batch.Config.Get by returning values dynamically adjusted for system load:
//   - At low load: larger batches (higher efficiency)
//   - At high load: smaller batches (reduced resource consumption)
//   - At low load: shorter wait times (lower latency)
//   - At high load: longer wait times (backpressure)
//
// This method is concurrency-safe and can be called from multiple goroutines.
func (c *loadBasedConfig) Get() batch.ConfigValues {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Scale batch size with load:
	// - Lower load = larger batches (more efficient)
	// - Higher load = smaller batches (less resource intensive)
	loadFactor := float64(100-c.currentLoad) / 100.0

	// Calculate dynamic values based on load
	minItems := uint64(float64(c.baseMinItems) * loadFactor)
	if minItems < 1 {
		minItems = 1
	}

	maxItems := uint64(float64(c.baseMaxItems) * loadFactor)
	if maxItems < minItems {
		maxItems = minItems
	}

	// For times, we do the opposite - longer times when load is high
	timeFactor := float64(c.currentLoad)/100.0 + 0.5 // range 0.5-1.5

	minTime := time.Duration(float64(c.baseMinTime) * timeFactor)
	maxTime := time.Duration(float64(c.baseMaxTime) * timeFactor)

	return batch.ConfigValues{
		MinItems: minItems,
		MaxItems: maxItems,
		MinTime:  minTime,
		MaxTime:  maxTime,
	}
}

// UpdateLoad changes the simulated system load value used to adjust batch parameters.
// Load values are clamped to the range 0-100, representing percentage of system load.
// This method is concurrency-safe and can be used to simulate changing system conditions.
func (c *loadBasedConfig) UpdateLoad(newLoad int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if newLoad < 0 {
		newLoad = 0
	} else if newLoad > 100 {
		newLoad = 100
	}

	c.currentLoad = newLoad
	fmt.Printf("System load changed to %d%%\n", newLoad)
}

// logConfig prints the current configuration values for demonstration purposes.
// This is a helper function used to show how configuration values adapt to system load.
func logConfig(config batch.ConfigValues) {
	fmt.Printf("Current config: MinItems=%d, MaxItems=%d, MinTime=%v, MaxTime=%v\n",
		config.MinItems, config.MaxItems, config.MinTime, config.MaxTime)
}

// batchInfoProcessor implements batch.Processor to log information about each
// batch as it's processed. It's used to demonstrate batch sizes in the example.
type batchInfoProcessor struct{}

// Process implements batch.Processor.Process by logging the batch size and
// returning the items unmodified.
func (p *batchInfoProcessor) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	fmt.Printf("Processing batch of %d items\n", len(items))
	return items, nil
}

// Example_customConfig demonstrates implementing a custom batch.Config that
// dynamically adjusts batch processing parameters based on system conditions.
//
// This example shows:
// 1. Creating a custom Config implementation that responds to system load
// 2. How batch parameters scale inversely with load (smaller batches under high load)
// 3. How timing parameters scale directly with load (longer times under high load)
// 4. A complete processing pipeline using the custom configuration
//
// In real-world applications, this pattern can be used to implement backpressure
// mechanisms that adapt to system resource availability.
func Example_customConfig() {
	// Create a custom config that adapts to system load
	cfg := newLoadBasedConfig(
		10,                   // Base MinItems
		50,                   // Base MaxItems
		200*time.Millisecond, // Base MinTime
		1*time.Second,        // Base MaxTime
	)

	// Log initial configuration
	fmt.Println("Initial configuration:")
	logConfig(cfg.Get())

	// Create batch processor with our custom config
	b := batch.New(cfg)
	processor := &batchInfoProcessor{}

	// Create a custom slice source with numbers
	nums := make([]interface{}, 200)
	for i := 0; i < 200; i++ {
		nums[i] = i
	}
	s := &sliceSource{items: nums}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start processing in background
	fmt.Println("Starting batch processing...")
	errs := b.Go(ctx, s, processor)

	// Wait for completion
	batch.IgnoreErrors(errs)
	<-b.Done()
	fmt.Println("Processing complete")

	// Output:
	// Initial configuration:
	// Current config: MinItems=5, MaxItems=25, MinTime=200ms, MaxTime=1s
	// Starting batch processing...
	// Processing batch of 25 items
	// Processing batch of 25 items
	// Processing batch of 25 items
	// Processing batch of 25 items
	// Processing batch of 25 items
	// Processing batch of 25 items
	// Processing batch of 25 items
	// Processing batch of 25 items
	// Processing complete
}
