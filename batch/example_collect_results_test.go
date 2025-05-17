package batch_test

import (
	"context"
	"fmt"

	"github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/processor"
	"github.com/MasterOfBinary/gobatch/source"
)

func Example_collectResults() {
	b := batch.New(batch.NewConstantConfig(&batch.ConfigValues{MinItems: 5, MaxItems: 5}))

	ch := make(chan interface{}, 5)
	for i := 1; i <= 5; i++ {
		ch <- i
	}
	close(ch)

	src := &source.Channel{Input: ch}
	doubleProc := &processor.Transform{Func: func(data interface{}) (interface{}, error) {
		if v, ok := data.(int); ok {
			return v * 2, nil
		}
		return data, nil
	}}

	collect := &processor.Collect{}
	ctx := context.Background()

	errs := batch.RunBatchAndWait(ctx, b, src, doubleProc, collect)

	fmt.Println(collect.Data)
	fmt.Println("errors:", len(errs))
	// Output:
	// [2 4 6 8 10]
	// errors: 0
}
