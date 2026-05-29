package batch_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/source"
)

type simpleProcessor struct{}

func (p *simpleProcessor) Process(ctx context.Context, items []*batch.Item[any]) ([]*batch.Item[any], error) {
	var values []any

	for _, item := range items {
		if val, ok := item.Data.(int); ok && val == 5 {
			item.Error = errors.New("value 5 not allowed")
			continue
		}
		values = append(values, item.Data)
	}

	fmt.Println("Batch:", values)
	return items, nil
}

func Example_simpleProcessor() {
	ch := make(chan any)

	go func() {
		for _, v := range []any{1, 2, 3, 4, 5, 6, 7, 8, 9, 10} {
			ch <- v
		}
		close(ch)
	}()

	src := &source.Channel[any]{Input: ch}

	// Collect all ten items into a single batch. Each batch is processed in its
	// own goroutine, so spreading the items over several batches would make the
	// "Batch:" lines print in a non-deterministic order. Requiring (and
	// capping) the batch at the full item count yields exactly one batch and a
	// stable output.
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 10,
		MaxItems: 10,
	})

	p := &simpleProcessor{}
	b := batch.New[any](config)

	ctx := context.Background()
	fmt.Println("Starting...")

	errs := batch.RunBatchAndWait[any](ctx, b, src, p)

	if len(errs) > 0 {
		fmt.Printf("Errors: %d\n", len(errs))
		fmt.Println("Last error:", errs[len(errs)-1])
	}

	// Output:
	// Starting...
	// Batch: [1 2 3 4 6 7 8 9 10]
	// Errors: 1
	// Last error: processor error: value 5 not allowed
}
