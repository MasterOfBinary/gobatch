// Package gobatch is the root of the GoBatch library.
//
// It consists of three subpackages:
//
//   - batch: The core batching engine for building processing pipelines.
//   - processor: Several Processor implementations for common operations.
//   - source: Source implementations for ingesting data from various origins.
//
// Basic usage ties these packages together:
//
//	cfg := batch.NewConstantConfig(&batch.ConfigValues{MinItems: 1})
//	b := batch.New(cfg)
//	ch := make(chan interface{}, 1)
//	ch <- "hello"
//	close(ch)
//	src := &source.Channel{Input: ch}
//	proc := &processor.Transform{Func: func(v interface{}) (interface{}, error) {
//	    fmt.Println(v)
//	    return v, nil
//	}}
//
//	batch.IgnoreErrors(b.Go(context.Background(), src, proc))
//	<-b.Done()
//
// Output:
//
// hello
//
// See the README.md for an overview of how these pieces fit together.
package gobatch
