package batch

import (
	"context"
	"sync"
)

// IgnoreErrors starts a goroutine that reads errors from errs but ignores them.
// It can be used with Batch.Go if errors aren't needed. Ignoring the returned
// channel without reading from it can block once the buffer fills. For example:
//
//	// NOTE: bad - leaving errs undrained can deadlock once the buffer fills!
//	errs, _ := myBatch.Go(ctx, s, p)
//	_ = errs
//
// Instead, IgnoreErrors can be used to safely discard all errors:
//
//	errs, err := myBatch.Go(ctx, s, p)
//	if err != nil {
//		log.Fatal(err)
//	}
//	batch.IgnoreErrors(errs)
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
// It blocks until the error channel is closed (i.e., until batch processing
// completes), so there is no need to wait on Done afterwards.
//
// Example usage:
//
//	pipeErrs, err := myBatch.Go(ctx, source, processor)
//	if err != nil {
//		log.Fatal(err)
//	}
//	errs := batch.CollectErrors(pipeErrs)
//	// CollectErrors blocks until processing is done.
//	for _, err := range errs {
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
//	errs := batch.RunBatchAndWait(ctx, myBatch, source, processor1, processor2)
//	if len(errs) > 0 {
//		// Handle errors
//	}
func RunBatchAndWait[T any](ctx context.Context, b *Batch[T], s Source[T], procs ...Processor[T]) []error {
	// Start the batch processing. A start error (e.g. ErrNilSource or
	// ErrBatchUsed) is surfaced in the returned slice so callers that only
	// inspect the slice still see the failure.
	errs, err := b.Go(ctx, s, procs...)
	if err != nil {
		return []error{err}
	}

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
type BatchConfig[T any] struct {
	B *Batch[T]      // The Batch instance to use
	S Source[T]      // The Source to read items from
	P []Processor[T] // The processors to apply to the items
}

// ExecuteBatches runs multiple batches concurrently and waits for all to complete.
// It returns all errors from all batches as a slice. This is useful when you need
// to process multiple data sources in parallel.
//
// Example usage:
//
//	errs := batch.ExecuteBatches(ctx,
//		&batch.BatchConfig[int]{B: batch1, S: source1, P: []batch.Processor[int]{proc1}},
//		&batch.BatchConfig[int]{B: batch2, S: source2, P: []batch.Processor[int]{proc2}},
//	)
func ExecuteBatches[T any](ctx context.Context, configs ...*BatchConfig[T]) []error {
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		allErrs []error
	)

	wg.Add(len(configs))

	for _, config := range configs {
		go func(cfg *BatchConfig[T]) {
			defer wg.Done()

			if cfg == nil || cfg.B == nil || cfg.S == nil {
				return
			}

			errs, err := cfg.B.Go(ctx, cfg.S, cfg.P...)
			if err != nil {
				mu.Lock()
				allErrs = append(allErrs, err)
				mu.Unlock()
				return
			}
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
