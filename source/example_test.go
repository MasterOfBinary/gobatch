package source_test

import (
	"context"
	"fmt"

	"github.com/MasterOfBinary/gobatch/source"
)

func ExampleChannel() {
	input := make(chan interface{}, 2)
	input <- "a"
	input <- "b"
	close(input)

	src := &source.Channel{Input: input}
	out, errs := src.Read(context.Background())
	for item := range out {
		fmt.Println(item)
	}
	for range errs {
	}
	// Output:
	// a
	// b
}
