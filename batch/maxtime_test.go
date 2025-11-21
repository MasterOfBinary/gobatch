package batch_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/processor"
	"github.com/MasterOfBinary/gobatch/source"
)

// TestMaxTimeIdle verifies that the MaxTime timer works correctly even if the batch
// remains empty for longer than MaxTime before the first item arrives.
// Previously, the timer would expire on an empty batch and not restart, causing
// the first item to wait indefinitely (or until MinItems).
func TestMaxTimeIdle(t *testing.T) {
	// Config: Wait up to 100ms, or 10 items.
	// We want to verify that an item arriving at T=200ms is processed by T=300ms.
	cfg := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 10,
		MaxTime:  100 * time.Millisecond,
	})

	b := batch.New(cfg)

	// Input channel
	input := make(chan interface{})
	src := &source.Channel{Input: input}

	// Counter processor
	var processedCount int32
	proc := &processor.Transform{
		Func: func(data interface{}) (interface{}, error) {
			atomic.AddInt32(&processedCount, 1)
			return data, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b.Go(ctx, src, proc)

	// 1. Wait for longer than MaxTime (200ms > 100ms) to trigger the idle expiration bug
	time.Sleep(200 * time.Millisecond)

	// 2. Send one item
	input <- 1

	// 3. Wait for MaxTime + buffer (200ms)
	// The item should be processed within 100ms of arrival.
	time.Sleep(200 * time.Millisecond)

	// Check result
	count := atomic.LoadInt32(&processedCount)
	if count == 0 {
		t.Fatalf("Item was not processed after MaxTime elapsed! The timer likely expired while idle and wasn't reset.")
	}

	// Cleanup
	close(input)
	<-b.Done()
}
