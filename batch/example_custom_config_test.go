package batch_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
)

type sliceSource struct {
	items []interface{}
	delay time.Duration
}

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
			}
		}
	}()

	return out, errs
}

type loadBasedConfig struct {
	mu          sync.RWMutex
	currentLoad int
	baseMin     uint64
	baseMax     uint64
	baseMinTime time.Duration
	baseMaxTime time.Duration
}

func newLoadBasedConfig(baseMin, baseMax uint64, minTime, maxTime time.Duration) *loadBasedConfig {
	return &loadBasedConfig{
		currentLoad: 50,
		baseMin:     baseMin,
		baseMax:     baseMax,
		baseMinTime: minTime,
		baseMaxTime: maxTime,
	}
}

func (c *loadBasedConfig) Get() batch.ConfigValues {
	c.mu.RLock()
	defer c.mu.RUnlock()

	loadFactor := float64(100-c.currentLoad) / 100.0
	minItems := uint64(float64(c.baseMin) * loadFactor)
	if minItems < 1 {
		minItems = 1
	}
	maxItems := uint64(float64(c.baseMax) * loadFactor)
	if maxItems < minItems {
		maxItems = minItems
	}

	timeFactor := float64(c.currentLoad)/100.0 + 0.5
	minTime := time.Duration(float64(c.baseMinTime) * timeFactor)
	maxTime := time.Duration(float64(c.baseMaxTime) * timeFactor)

	return batch.ConfigValues{
		MinItems: minItems,
		MaxItems: maxItems,
		MinTime:  minTime,
		MaxTime:  maxTime,
	}
}

func (c *loadBasedConfig) UpdateLoad(load int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if load < 0 {
		load = 0
	} else if load > 100 {
		load = 100
	}

	c.currentLoad = load
	fmt.Printf("System load set to %d%%\n", load)
}

type batchInfoProcessor struct{}

func (p *batchInfoProcessor) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	fmt.Printf("Batch of %d items\n", len(items))
	return items, nil
}

func Example_customConfig() {
	cfg := newLoadBasedConfig(
		10,                   // base min items
		50,                   // base max items
		200*time.Millisecond, // base min time
		1*time.Second,        // base max time
	)

	fmt.Println("=== Custom Config Example ===")
	fmt.Println("Initial config:")
	printConfig(cfg.Get())

	b := batch.New(cfg)
	p := &batchInfoProcessor{}

	nums := make([]interface{}, 200)
	for i := range nums {
		nums[i] = i
	}
	src := &sliceSource{items: nums}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Println("Starting batch processing...")
	errs := b.Go(ctx, src, p)

	batch.IgnoreErrors(errs)
	<-b.Done()
	fmt.Println("Processing complete")

	// Output:
	// === Custom Config Example ===
	// Initial config:
	// MinItems=5, MaxItems=25, MinTime=200ms, MaxTime=1s
	// Starting batch processing...
	// Batch of 25 items
	// Batch of 25 items
	// Batch of 25 items
	// Batch of 25 items
	// Batch of 25 items
	// Batch of 25 items
	// Batch of 25 items
	// Batch of 25 items
	// Processing complete
}

func printConfig(c batch.ConfigValues) {
	fmt.Printf("MinItems=%d, MaxItems=%d, MinTime=%v, MaxTime=%v\n",
		c.MinItems, c.MaxItems, c.MinTime, c.MaxTime)
}
