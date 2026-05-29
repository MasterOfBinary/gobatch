package batch_test

import (
	"context"
	"fmt"

	"github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/processor"
	"github.com/MasterOfBinary/gobatch/source"
)

func Example() {
	// Create a batch processor that collects all five items into a single
	// batch. Because every batch is processed in its own goroutine, splitting
	// the items across multiple batches would let those goroutines print in a
	// non-deterministic order. Requiring MinItems items before processing (and
	// capping MaxItems at the same value) guarantees exactly one batch, so the
	// output below is stable.
	b := batch.New[int](batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 5,
		MaxItems: 5,
	}))

	// Create an input channel
	ch := make(chan int)

	// Wrap it with source.Channel
	src := &source.Channel[int]{Input: ch}

	// First processor: double the value
	doubleProc := &processor.Transform[int]{
		Func: func(data int) (int, error) {
			return data * 2, nil
		},
	}

	// Second processor: print the result
	printProc := &processor.Transform[int]{
		Func: func(data int) (int, error) {
			fmt.Println(data)
			return data, nil
		},
	}

	ctx := context.Background()

	// Start processing with both processors chained
	errs, err := b.Go(ctx, src, doubleProc, printProc)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Ignore errors
	batch.IgnoreErrors(errs)

	// Send some items
	go func() {
		for i := 1; i <= 5; i++ {
			ch <- i
		}
		close(ch)
	}()

	// Wait for completion
	<-b.Done()

	// Output:
	// 2
	// 4
	// 6
	// 8
	// 10
}
