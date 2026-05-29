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
//	b := batch.New[string](cfg)
//	ch := make(chan string, 1)
//	ch <- "hello"
//	close(ch)
//	src := &source.Channel[string]{Input: ch}
//	proc := &processor.Transform[string]{Func: func(v string) (string, error) {
//	    fmt.Println(v)
//	    return v, nil
//	}}
//
//	errs, err := b.Go(context.Background(), src, proc)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	batch.IgnoreErrors(errs)
//	<-b.Done()
//
// The Transform func above prints each item, so this pipeline writes "hello".
//
// See the README.md for an overview of how these pieces fit together.
package gobatch
