package processor_test

import (
	"context"
	"fmt"

	"github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/processor"
)

func ExampleTransform() {
	p, _ := processor.NewTransform(processor.TransformConfig{
		Func: func(v interface{}) (interface{}, error) {
			if n, ok := v.(int); ok {
				return n * 2, nil
			}
			return v, nil
		},
	})

	items := []*batch.Item{{Data: 1}, {Data: 2}}
	res, _ := p.Process(context.Background(), items)
	fmt.Println(res[0].Data, res[1].Data)
	// Output:
	// 2 4
}
