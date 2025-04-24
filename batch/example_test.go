package batch_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/source"
)

// printProcessor is a Processor that prints items in batches.
// To demonstrate how errors can be handled, it fails to process the number 5.
type printProcessor struct{}

// Process prints a batch of items and marks item 5 as failed.
func (p printProcessor) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	toPrint := make([]interface{}, 0, len(items))
	for _, item := range items {
		if num, ok := item.Data.(int); ok && num == 5 {
			item.Error = errors.New("cannot process 5")
			continue
		}
		toPrint = append(toPrint, item.Data)
	}
	fmt.Println(toPrint)
	return items, nil
}

func Example() {
	// Create a batch processor that processes items 5 at a time
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 5,
	})
	b := batch.New(config)
	p := &printProcessor{}

	// Channel is a Source that reads from a channel until it's closed
	ch := make(chan interface{})
	s := source.Channel{
		Input: ch,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errs := b.Go(ctx, &s, p)

	go func() {
		for i := 0; i < 20; i++ {
			time.Sleep(time.Millisecond * 10)
			ch <- i
		}
		close(ch)
	}()

	var lastErr error
	for err := range errs {
		lastErr = err
	}

	fmt.Println("Finished processing.")
	if lastErr != nil {
		fmt.Println("Found error:", lastErr.Error())
	}
	// Output:
	// [0 1 2 3 4]
	// [6 7 8 9]
	// [10 11 12 13 14]
	// [15 16 17 18 19]
	// Finished processing.
	// Found error: processor error: cannot process 5
}

// stringSource is a custom Source that generates string data.
type stringSource struct {
	strings []string
}

// Read implements the Source interface by sending strings to the output channel.
func (s *stringSource) Read(ctx context.Context) (<-chan interface{}, <-chan error) {
	out := make(chan interface{})
	errs := make(chan error)

	go func() {
		defer close(out)
		defer close(errs)

		for _, str := range s.strings {
			select {
			case <-ctx.Done():
				return
			case out <- str:
				// String sent successfully
				time.Sleep(time.Millisecond * 5) // Simulate some processing time
			}
		}
	}()

	return out, errs
}

// uppercaseProcessor is a custom Processor that converts strings to uppercase.
type uppercaseProcessor struct{}

// Process implements the Processor interface by converting strings to uppercase.
func (p *uppercaseProcessor) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	processed := make([]interface{}, 0, len(items))

	for _, item := range items {
		// Skip items that already have errors
		if item.Error != nil {
			continue
		}

		// Check for context cancellation
		select {
		case <-ctx.Done():
			return items, ctx.Err()
		default:
			// Continue processing
		}

		// Process only string data
		if str, ok := item.Data.(string); ok {
			item.Data = fmt.Sprintf("[%s]", str)
			processed = append(processed, item.Data)
		} else {
			item.Error = errors.New("not a string")
		}
	}

	if len(processed) > 0 {
		fmt.Printf("Processed batch: %v\n", processed)
	}

	return items, nil
}

// filterShortStringsProcessor filters out strings shorter than specified length
type filterShortStringsProcessor struct {
	minLength int
}

// Process implements the Processor interface by filtering short strings
func (p *filterShortStringsProcessor) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	result := make([]*batch.Item, 0, len(items))
	filtered := make([]string, 0)

	for _, item := range items {
		// Skip items that already have errors
		if item.Error != nil {
			result = append(result, item)
			continue
		}

		// Filter only string data
		if str, ok := item.Data.(string); ok {
			if len(str) >= p.minLength {
				result = append(result, item)
			} else {
				filtered = append(filtered, str)
			}
		} else {
			// Keep non-string items
			result = append(result, item)
		}
	}

	if len(filtered) > 0 {
		fmt.Printf("Filtered out short strings: %v\n", filtered)
	}

	return result, nil
}

func Example_customSourceAndProcessor() {
	// Create custom source with string data
	source := &stringSource{
		strings: []string{"hello", "world", "go", "batch", "processing", "is", "fun"},
	}

	// Create batch processor with custom config
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 2,                     // Process at least 2 items at once
		MaxItems: 3,                     // Process at most 3 items at once
		MinTime:  10 * time.Millisecond, // Wait at least 10ms before processing
	})

	// Create processor chain
	uppercaseProc := &uppercaseProcessor{}
	filterProc := &filterShortStringsProcessor{minLength: 4}

	// Create batch processor
	batchProcessor := batch.New(config)

	// Setup context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Println("Starting custom source and processor example...")

	// Use the helper function to avoid race conditions by waiting for completion
	errors := batch.RunBatchAndWait(ctx, batchProcessor, source, filterProc, uppercaseProc)

	fmt.Println("Processing complete")
	if len(errors) > 0 {
		fmt.Println("Errors occurred during processing:")
		for _, err := range errors {
			fmt.Println("-", err)
		}
	}

	// Output:
	// Starting custom source and processor example...
	// Filtered out short strings: [go]
	// Processed batch: [[hello] [world]]
	// Filtered out short strings: [is]
	// Processed batch: [[batch]]
	// Filtered out short strings: [fun]
	// Processed batch: [[processing]]
	// Processing complete
}
