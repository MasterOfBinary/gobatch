package batch_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/source"
)

type simpleProcessor struct{}

func (p *simpleProcessor) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	var values []interface{}

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
	ch := make(chan interface{})

	go func() {
		for _, v := range []interface{}{1, 2, 3, 4, 5, 6, 7, 8, 9, 10} {
			ch <- v
			time.Sleep(10 * time.Millisecond)
		}
		close(ch)
	}()

	src := &source.Channel{Input: ch}

	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 5,
		MaxItems: 3,
	})

	p := &simpleProcessor{}
	b := batch.New(config)

	ctx := context.Background()
	fmt.Println("Starting...")

	errs := batch.RunBatchAndWait(ctx, b, src, p)

	if len(errs) > 0 {
		fmt.Printf("Errors: %d\n", len(errs))
		fmt.Println("Last error:", errs[len(errs)-1])
	}

	// Output:
	// Starting...
	// Batch: [1 2 3]
	// Batch: [4 6]
	// Batch: [7 8 9]
	// Batch: [10]
	// Errors: 1
	// Last error: processor error: value 5 not allowed
}
