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
	b := batch.New(batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 2,
		MaxItems: 5,
		MinTime:  10 * time.Millisecond,
		MaxTime:  100 * time.Millisecond,
	}))

	// Create an input channel
	ch := make(chan interface{})

	// Wrap it with source.Channel
	src := &source.Channel{Input: ch}

	// First processor: double the value
	doubleProc := &processor.Transform{
		Func: func(data interface{}) (interface{}, error) {
			if v, ok := data.(int); ok {
				return v * 2, nil
			}
			return data, nil
		},
	}

	// Second processor: print the result
	printProc := &processor.Transform{
		Func: func(data interface{}) (interface{}, error) {
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
