package batch_test

import (
	"context"
	"fmt"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/processor"
	"github.com/MasterOfBinary/gobatch/source"
)

func Example() {
	// Create a batch processor with simple config
	b := batch.New[int](batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 2,
		MaxItems: 5,
		MinTime:  10 * time.Millisecond,
		MaxTime:  100 * time.Millisecond,
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
	errs := b.Go(ctx, src, doubleProc, printProc)

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
