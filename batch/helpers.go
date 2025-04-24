package batch

import (
	"context"
	"sync"
)

// IgnoreErrors starts a goroutine that reads errors from errs but ignores them.
// It can be used with Batch.Go if errors aren't needed. Since the error channel
// is unbuffered, one cannot just throw away the error channel like this:
//
//	// NOTE: bad - this can cause a deadlock!
//	_ = batch.Go(ctx, p, s)
//
// Instead, IgnoreErrors can be used to safely throw away all errors:
//
//	batch.IgnoreErrors(myBatch.Go(ctx, p, s))
func IgnoreErrors(errs <-chan error) {
	// nil channels always block, so check for nil first to avoid a goroutine
	// leak
	if errs != nil {
		go func() {
			for range errs {
			}
		}()
	}
}

// CollectErrors collects all errors from the error channel into a slice.
// This is useful when you need to process all errors after batch processing completes.
//
// Example usage:
//
//	errors := batch.CollectErrors(myBatch.Go(ctx, source, processor))
//	<-myBatch.Done()
//	for _, err := range errors {
//		log.Printf("Error: %v", err)
//	}
func CollectErrors(errs <-chan error) []error {
	if errs == nil {
		return nil
	}

	var result []error
	for err := range errs {
		result = append(result, err)
	}
	return result
}

// RunBatchAndWait is a convenience function that runs a batch with the given source
// and processors, waits for it to complete, and returns all errors encountered.
// This is useful for simple batch processing where you don't need to
// handle errors asynchronously.
//
// Example usage:
//
//	errors := batch.RunBatchAndWait(ctx, myBatch, source, processor1, processor2)
//	if len(errors) > 0 {
//		// Handle errors
//	}
func RunBatchAndWait(ctx context.Context, b *Batch, s Source, procs ...Processor) []error {
	// Start the batch processing
	errs := b.Go(ctx, s, procs...)

	// Collect all errors into a slice
	var collectedErrors []error
	for err := range errs {
		if err != nil {
			collectedErrors = append(collectedErrors, err)
		}
	}

	// Wait for completion
	<-b.Done()

	return collectedErrors
}

// BatchConfig holds the configuration for a single batch execution.
// It combines a Batch instance, a Source to read from, and a list of Processors
// to apply to the data from the source. This is used primarily with the
// ExecuteBatches function to run multiple batch operations concurrently.
type BatchConfig struct {
	B *Batch      // The Batch instance to use
	S Source      // The Source to read items from
	P []Processor // The processors to apply to the items
}

// ExecuteBatches runs multiple batches concurrently and waits for all to complete.
// It returns all errors from all batches as a slice. This is useful when you need
// to process multiple data sources in parallel.
//
// Example usage:
//
//	errors := batch.ExecuteBatches(ctx,
//		&batch.BatchConfig{B: batch1, S: source1, P: []Processor{proc1}},
//		&batch.BatchConfig{B: batch2, S: source2, P: []Processor{proc2}},
//	)
func ExecuteBatches(ctx context.Context, configs ...*BatchConfig) []error {
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		allErrs []error
	)

	wg.Add(len(configs))

	for _, config := range configs {
		go func(cfg *BatchConfig) {
			defer wg.Done()

			if cfg.B == nil || cfg.S == nil {
				return
			}

			errs := cfg.B.Go(ctx, cfg.S, cfg.P...)
			for err := range errs {
				mu.Lock()
				allErrs = append(allErrs, err)
				mu.Unlock()
			}

			<-cfg.B.Done()
		}(config)
	}

	wg.Wait()
	return allErrs
}
