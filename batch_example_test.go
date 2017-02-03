package gobatch_test

import (
	"fmt"

	"github.com/MasterOfBinary/gobatch"
)

func ExampleMust() {
	defer func() {
		if p := recover(); p != nil {
			fmt.Println("Panic!")
		}
	}()

	b := gobatch.Must(gobatch.NewBuilder().
		WithReadConcurrency(0).
		Batch())

	// Do something with b...
	_ = b
	// Output: Panic!
}
