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

	badConfig := &gobatch.BatchConfig{
		MinItems: 5,
		MaxItems: 1,
	}
	b := gobatch.Must(gobatch.New(badConfig))

	// Do something with b...
	_ = b
	// Output: Panic!
}
