package batch_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
)

// errorSource demonstrates a source that produces both data and errors.
// It sends integer items to the output channel and periodically
// generates errors based on the specified error rate.
type errorSource struct {
	items     []int
	errorRate int // Introduce an error every N items
}

// Read implements the Source interface by sending items to the output channel
// and periodically producing errors based on the errorRate. It respects context
// cancellation and properly closes both channels when done.
func (s *errorSource) Read(ctx context.Context) (<-chan interface{}, <-chan error) {
	out := make(chan interface{})
	errs := make(chan error)

	go func() {
		defer close(out)
		defer close(errs)

		for i, val := range s.items {
			select {
			case <-ctx.Done():
				errs <- fmt.Errorf("source interrupted: %w", ctx.Err())
				return
			default:
				// Occasionally generate a source error
				if s.errorRate > 0 && i > 0 && i%s.errorRate == 0 {
					errs <- fmt.Errorf("source error at item %d", i)
					time.Sleep(time.Millisecond * 10) // Slight delay
					continue
				}

				// Send the item
				out <- val
				time.Sleep(time.Millisecond * 5)
			}
		}
	}()

	return out, errs
}

// validationProcessor validates items and sets per-item errors for invalid ones.
// It marks items that exceed the specified maximum value as errors.
type validationProcessor struct {
	maxValue int
}

// Process implements the Processor interface by validating each item against
// a maximum value threshold. Items exceeding the threshold are marked with errors.
// It respects context cancellation and skips items that already have errors.
func (p *validationProcessor) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	for _, item := range items {
		// Skip items that already have errors
		if item.Error != nil {
			continue
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return items, ctx.Err()
		default:
		}

		// Type assertion and validation
		if num, ok := item.Data.(int); ok {
			if num > p.maxValue {
				item.Error = fmt.Errorf("value %d exceeds maximum %d", num, p.maxValue)
			}
		} else {
			item.Error = errors.New("expected int type")
		}
	}

	return items, nil
}

// errorProneProcessor simulates a processor that may fail with a
// processor-wide error. It also transforms integers to string representations.
type errorProneProcessor struct {
	failOnBatch int
	batchCount  int
}

// Process implements the Processor interface by transforming integers to strings
// and deliberately failing on a specific batch. This demonstrates how processor-wide
// failures are handled differently from per-item errors.
func (p *errorProneProcessor) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	p.batchCount++

	// Simulate a processor-wide failure on specific batch
	if p.batchCount == p.failOnBatch {
		return items, fmt.Errorf("processor failed on batch %d", p.batchCount)
	}

	// Process items normally
	for _, item := range items {
		// Skip items that already have errors
		if item.Error != nil {
			continue
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return items, ctx.Err()
		default:
		}

		// Convert to string
		if num, ok := item.Data.(int); ok {
			item.Data = "Item: " + strconv.Itoa(num)
		}
	}

	return items, nil
}

// errorLogger is a simple processor that logs items and their errors.
// It displays item data and error information to demonstrate error tracking.
type errorLogger struct{}

// Process implements the Processor interface by displaying the current state
// of each item, including any errors. It counts and reports the number of
// errors in each batch but does not modify the items.
func (p *errorLogger) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	fmt.Println("Processing batch...")
	errorCount := 0

	for _, item := range items {
		if item.Error != nil {
			fmt.Printf("- Item %d has error: %v\n", item.ID, item.Error)
			errorCount++
		} else {
			fmt.Printf("- Item %d: %v\n", item.ID, item.Data)
		}
	}

	if errorCount > 0 {
		fmt.Printf("Batch contains %d items with errors\n", errorCount)
	}

	return items, nil
}

func Example_errorHandling() {
	// Create a source that generates data and occasional errors
	source := &errorSource{
		items:     []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 15, 20, 25},
		errorRate: 5, // Introduce a source error every 5 items
	}

	// Create a validation processor that fails on values > 10
	validator := &validationProcessor{maxValue: 10}

	// Create a processor that fails on the 2nd batch
	transformer := &errorProneProcessor{failOnBatch: 2}

	// Create a processor that logs the items and their errors
	logger := &errorLogger{}

	// Create a batch processor with basic configuration
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 3,
		MaxItems: 5,
	})
	b := batch.New(config)

	// Context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fmt.Println("Starting error handling example...")

	// Start the batch processing
	errs := batch.RunBatchAndWait(ctx, b, source, validator, transformer, logger)

	fmt.Println("\nProcessing complete. Error summary:")
	if len(errs) == 0 {
		fmt.Println("No errors occurred")
	} else {
		for i, err := range errs {
			var srcErr *batch.SourceError
			var procErr *batch.ProcessorError

			if errors.As(err, &srcErr) {
				fmt.Printf("%d. Source error: %v\n", i+1, srcErr.Unwrap())
			} else if errors.As(err, &procErr) {
				fmt.Printf("%d. Processor error: %v\n", i+1, procErr.Unwrap())
			} else {
				fmt.Printf("%d. Other error: %v\n", i+1, err)
			}
		}
	}

	// Output:
	// Starting error handling example...
	// Processing batch...
	// - Item 0: Item: 1
	// - Item 1: Item: 2
	// - Item 2: Item: 3
	// Processing batch...
	// - Item 3: 4
	// - Item 4: 5
	// - Item 5: 7
	// Processing batch...
	// - Item 6: Item: 8
	// - Item 7: Item: 9
	// - Item 8: Item: 10
	// Processing batch...
	// - Item 9 has error: value 12 exceeds maximum 10
	// - Item 10 has error: value 15 exceeds maximum 10
	// - Item 11 has error: value 20 exceeds maximum 10
	// Batch contains 3 items with errors
	// Processing batch...
	// - Item 12 has error: value 25 exceeds maximum 10
	// Batch contains 1 items with errors
	//
	// Processing complete. Error summary:
	// 1. Source error: source error at item 5
	// 2. Processor error: processor failed on batch 2
	// 3. Source error: source error at item 10
	// 4. Processor error: value 12 exceeds maximum 10
	// 5. Processor error: value 15 exceeds maximum 10
	// 6. Processor error: value 20 exceeds maximum 10
	// 7. Processor error: value 25 exceeds maximum 10
}
