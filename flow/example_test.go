package flow_test

import (
	"context"
	"fmt"

	"github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/flow"
	"github.com/MasterOfBinary/gobatch/processor"
	"github.com/MasterOfBinary/gobatch/source"
)

func Example() {
	input := 6               // Immutable shared input.
	var doubled, squared int // Separate branch-owned outputs.
	err := flow.Execute(context.Background(),
		flow.Parallel(
			func(context.Context) error { doubled = input * 2; return nil },
			func(context.Context) error { squared = input * input; return nil },
		),
		flow.Sequence(
			func(context.Context) error { fmt.Println(doubled, squared); return nil },
			func(context.Context) error { fmt.Println("joined"); return nil },
		),
	)
	fmt.Println(err)
	// Output:
	// 12 36
	// joined
	// <nil>
}

func ExampleExecute_batchDependency() {
	var values []int
	writeBatch := func(ctx context.Context) error {
		input := make(chan int, 2)
		input <- 3
		input <- 5
		close(input)
		// Every invocation owns a fresh Batch and drains/joins it before returning.
		b := batch.New[int](nil).WithExecutionConfig(batch.ExecutionConfig{ProcessorFailurePolicy: batch.StopChain})
		failures := batch.RunBatchAndWait[int](ctx, b, &source.Channel[int]{Input: input},
			&processor.Transform[int]{Func: func(v int) (int, error) { values = append(values, v); return v, nil }},
		)
		// A real application retains all bounded failures before selecting one.
		if len(failures) != 0 {
			return failures[0]
		}
		return nil
	}
	err := flow.Execute(context.Background(), writeBatch,
		func(context.Context) error { fmt.Println("completed:", values); return nil },
	)
	fmt.Println(err)
	// Output:
	// completed: [3 5]
	// <nil>
}
