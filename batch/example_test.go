package batch_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/source"
)

// simpleProcessor demonstrates basic processor functionality
type simpleProcessor struct{}

func (p *simpleProcessor) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	// Create a slice to hold values for printing
	values := make([]interface{}, 0, len(items))

	for _, item := range items {
		// Example rule: mark item with value 5 as error
		if val, ok := item.Data.(int); ok && val == 5 {
			item.Error = errors.New("value 5 not allowed")
			continue
		}
		values = append(values, item.Data)
	}

	// Print the batch of valid values
	fmt.Println("Processed batch:", values)
	return items, nil
}

func Example() {
	// Create data to process
	data := []interface{}{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	// Create a channel for data
	ch := make(chan interface{})

	// Feed data into the channel with delays in a goroutine
	go func() {
		for _, item := range data {
			ch <- item
			time.Sleep(time.Millisecond * 10) // 10ms delay between items to ensure deterministic batching
		}
		close(ch)
	}()

	// Create a slice-based source
	source := &source.Channel{
		Input: ch,
	}

	// Create a batch processor with configuration that demonstrates priority order
	// According to batch.go: MaxTime = MaxItems > EOF > MinTime > MinItems
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 5, // This would collect more items if it had higher priority
		MaxItems: 3, // This takes precedence over MinItems
	})
	p := &simpleProcessor{}
	b := batch.New(config)

	// Create and run the batch processor
	ctx := context.Background()
	fmt.Println("Starting batch processing...")
	errs := batch.RunBatchAndWait(ctx, b, source, p)

	// Report any errors
	if len(errs) > 0 {
		fmt.Printf("Found %d errors\n", len(errs))
		fmt.Println("Last error:", errs[len(errs)-1])
	}

	// Output:
	// Starting batch processing...
	// Processed batch: [1 2 3]
	// Processed batch: [4 6]
	// Processed batch: [7 8 9]
	// Processed batch: [10]
	// Found 1 errors
	// Last error: processor error: value 5 not allowed
}
