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
