package gobatch_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/MasterOfBinary/gobatch"
	"github.com/MasterOfBinary/gobatch/source"
)

type printProcessor struct{}

func (p printProcessor) Process(ctx context.Context, items []interface{}, errs chan<- error) {
	// Process needs to close the error channel after it's done
	defer close(errs)

	// This processor prints all the items in a line. If items includes 5 it throws
	// an error for no reason
	for i := 0; i < len(items); i++ {
		if items[i] == 5 {
			errs <- errors.New("Cannot process 5")
			return
		}
	}

	fmt.Println(items)
}

func Example() {
	b := gobatch.Must(gobatch.NewBuilder().Batch())
	p := &printProcessor{}

	// source.Channel reads from a channel until it's closed
	ch := make(chan interface{})
	s := source.Channel(ch)

	// Go runs in the background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errs := b.Go(ctx, s, p)

	go func() {
		for i := 0; i < 10; i++ {
			ch <- i
		}
		close(ch)
	}()

	// Wait for errors. When the error channel is closed the pipeline has been
	// completely drained
	var lastErr error
	for err := range errs {
		lastErr = err
	}

	fmt.Println("Finished processing.")
	if lastErr != nil {
		fmt.Println("Found error:", lastErr.Error())
	}

	// Output:
	// [0]
	// [1]
	// [2]
	// [3]
	// [4]
	// [6]
	// [7]
	// [8]
	// [9]
	// Finished processing.
	// Found error: Cannot process 5
}
