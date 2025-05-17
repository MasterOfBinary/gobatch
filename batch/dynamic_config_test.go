package batch_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/MasterOfBinary/gobatch/batch"
)

func TestBatch_DynamicConfiguration(t *testing.T) {
	t.Run("dynamic config updates during processing", func(t *testing.T) {
		// Start with MinItems: 50 to hold processing
		dynamicCfg := NewDynamicConfig(&ConfigValues{
			MinItems: 50,
			MaxItems: 0,
			MinTime:  0,
			MaxTime:  0,
		})

		batch := New(dynamicCfg)

		// Create items
		const totalItems = 100
		items := make([]interface{}, totalItems)
		for i := 0; i < totalItems; i++ {
			items[i] = i
		}

		src := &TestSource{Items: items, Delay: 5 * time.Millisecond}

		// Use separate atomic counters instead of changing the pointer
		var beforeConfigChange uint32
		var afterConfigChange uint32

		// Use a new processor type that's safe for this test
		proc := &TestProcessor{
			ProcessFn: func(ctx context.Context, items []*Item) ([]*Item, error) {
				// Add a delay to ensure the processing happens over time
				time.Sleep(10 * time.Millisecond)

				// Check if config has changed and increment appropriate counter
				config := dynamicCfg.Get()
				if config.MinItems == 50 {
					atomic.AddUint32(&beforeConfigChange, uint32(len(items)))
				} else {
					atomic.AddUint32(&afterConfigChange, uint32(len(items)))
				}

				return items, nil
			},
		}

		// Start batch processing with initial config
		errs := batch.Go(context.Background(), src, proc)

		// Wait a bit for some items to be read, but not processed due to MinItems: 50
		time.Sleep(100 * time.Millisecond)

		// Update config to release the items for processing
		dynamicCfg.UpdateBatchSize(5, 10)

		// Wait for completion
		<-batch.Done()

		// Drain errors
		for range errs {
			// Just drain
		}

		// With initial MinItems: 50, we expect no items processed initially
		if beforeConfigChange > 0 {
			t.Errorf("expected 0 items before config update, got %d", beforeConfigChange)
		}

		// After changing to MinItems: 5, we expect items to be processed
		if afterConfigChange == 0 {
			t.Error("expected items to be processed after config update, got 0")
		}
	})
}
