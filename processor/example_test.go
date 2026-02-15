package processor_test

import (
	"context"
	"fmt"

	"github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/processor"
)

func ExampleTransform() {
	p := &processor.Transform[int]{Func: func(v int) (int, error) {
		return v * 2, nil
	}}

	items := []*batch.Item[int]{{Data: 1}, {Data: 2}}
	res, _ := p.Process(context.Background(), items)
	fmt.Println(res[0].Data, res[1].Data)
	// Output:
	// 2 4
}
