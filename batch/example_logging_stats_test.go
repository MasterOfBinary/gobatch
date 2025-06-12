package batch_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/processor"
	"github.com/MasterOfBinary/gobatch/source"
)

func ExampleBatch_loggingAndStats() {
	// Create a channel source with some test data
	ch := make(chan interface{})
	go func() {
		for i := 1; i <= 10; i++ {
			ch <- i
			time.Sleep(5 * time.Millisecond)
		}
		close(ch)
	}()

	// Create a simple processor that fails on specific values
	proc := &processor.Transform{
		Func: func(data interface{}) (interface{}, error) {
			val := data.(int)
			if val%3 == 0 {
				return nil, errors.New("value is divisible by 3")
			}
			return val * 2, nil
		},
		ContinueOnError: true,
	}

	// Set up logging and statistics
	logger := batch.NewSimpleLogger(batch.LogLevelInfo)
	stats := batch.NewBasicStatsCollector()

	// Create batch with logging and stats
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 3,
		MaxItems: 5,
		MaxTime:  50 * time.Millisecond,
	})

	b := batch.New(config).
		WithLogger(logger).
		WithStats(stats)

	// Create source
	src := &source.Channel{Input: ch}

	// Run batch processing
	ctx := context.Background()
	errs := b.Go(ctx, src, proc)

	// Collect errors
	var errorCount int
	for range errs {
		errorCount++
	}

	// Wait for completion
	<-b.Done()

	// Display statistics
	finalStats := stats.GetStats()
	fmt.Printf("\n=== Statistics ===\n")
	fmt.Printf("Batches: %d started, %d completed\n",
		finalStats.BatchesStarted, finalStats.BatchesCompleted)
	fmt.Printf("Items: %d processed, %d errors (%.1f%% error rate)\n",
		finalStats.ItemsProcessed, finalStats.ItemErrors, finalStats.ErrorRate())
	fmt.Printf("Average batch size: %.1f\n", finalStats.AverageBatchSize())
	fmt.Printf("Average batch time: %v\n", finalStats.AverageBatchTime())
	fmt.Printf("Total errors: %d\n", errorCount)

	// The output will show batch processing progress and statistics.
	// Output format is not checked to avoid timing-related test failures.
}

func ExampleLoggingProcessor() {
	// Create a source with test data
	ch := make(chan interface{}, 5)
	for i := 1; i <= 5; i++ {
		ch <- i
	}
	close(ch)

	// Create processors
	multiplyProc := &processor.Transform{
		Func: func(data interface{}) (interface{}, error) {
			return data.(int) * 2, nil
		},
	}

	filterProc := &processor.Filter{
		Predicate: func(item *batch.Item) bool {
			val := item.Data.(int)
			return val > 5
		},
	}

	// Wrap processors with logging
	logger := batch.NewSimpleLogger(batch.LogLevelDebug)
	loggedMultiply := processor.WrapWithLogging(multiplyProc, logger, "Multiply")
	loggedFilter := processor.WrapWithLogging(filterProc, logger, "Filter")

	// Create batch
	b := batch.New(nil).WithLogger(logger)
	src := &source.Channel{Input: ch}

	// Process
	ctx := context.Background()
	batch.IgnoreErrors(b.Go(ctx, src, loggedMultiply, loggedFilter))
	<-b.Done()

	// Output contains debug logging showing processor execution details
}

func ExampleBasicStatsCollector() {
	// Create test data
	ch := make(chan interface{}, 10)
	for i := 1; i <= 10; i++ {
		ch <- i
	}
	close(ch)

	// Create a processor that simulates work
	workProc := &processor.Transform{
		Func: func(data interface{}) (interface{}, error) {
			// Simulate some work
			time.Sleep(time.Millisecond)
			return data, nil
		},
	}

	// Wrap with statistics collection
	stats := batch.NewBasicStatsCollector()
	statsProc := processor.WrapWithStats(workProc, stats, true)

	// Create and run batch
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 5,
		MaxItems: 5,
	})
	b := batch.New(config)
	src := &source.Channel{Input: ch}

	ctx := context.Background()
	batch.IgnoreErrors(b.Go(ctx, src, statsProc))
	<-b.Done()

	// Display processor-specific statistics
	procStats := stats.GetStats()
	fmt.Printf("Processor stats:\n")
	fmt.Printf("- Batches processed: %d\n", procStats.BatchesCompleted)
	fmt.Printf("- Items processed: %d\n", procStats.ItemsProcessed)
	fmt.Printf("- Min batch time: %v\n", procStats.MinBatchTime)
	fmt.Printf("- Max batch time: %v\n", procStats.MaxBatchTime)

	// The output will show processor-specific statistics.
	// Actual timing values may vary but should be close to the sleep time.
	// Output format is not checked to avoid timing-related test failures.
}
