package gobatch_test

import (
	"context"
	"fmt"

	"github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/processor"
	"github.com/MasterOfBinary/gobatch/source"
)

func Example() {
	cfg := batch.NewConstantConfig(&batch.ConfigValues{MinItems: 1})
	b := batch.New[string](cfg)

	ch := make(chan string, 1)
	ch <- "hello"
	close(ch)

	src := &source.Channel[string]{Input: ch}
	proc := &processor.Transform[string]{Func: func(v string) (string, error) {
		fmt.Println(v)
		return v, nil
	}}

	batch.IgnoreErrors(b.Go(context.Background(), src, proc))
	<-b.Done()
	// Output:
	// hello
}
