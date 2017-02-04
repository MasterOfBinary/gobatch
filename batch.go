package gobatch

import (
	"context"
	"sync"
	"time"

	"github.com/MasterOfBinary/gobatch/processor"
	"github.com/MasterOfBinary/gobatch/source"
)

// Batch provides batch processing given an Source and a Processor. Data is
// read from the Source and processed in batches by the Processor. Any errors
// are wrapped in either a SourceError or a ProcessorError, so the caller
// can determine where the errors came from.
//
// To create a new Batch, call the New function. Creating one using &Batch{}
// will return the default Batch.
//
//    // The following are equivalent
//    defaultBatch1 := &gobatch.Batch{}
//    defaultBatch2 := gobatch.New(nil)
//    defaultBatch3 := gobatch.New(NewConstantBatchConfig(&NewConstantBatchConfig()))
//
// The defaults (with nil BatchConfig) provide a usable, but likely suboptimal, Batch
// where items are processed as soon as they are retrieved from the source. Reading
// is done by a single goroutine, and processing is done in the background using as
// many goroutines as necessary with no limit.
//
// This is a simplified version of how the default Batch works:
//
//    items := make(chan interface{})
//    errs := make(chan error)
//    go source.Read(ctx, items, errs)
//    for item := range items {
//      go processor.Process(ctx, []interface{}{item}, errs)
//    }
//
// Batch runs asynchronously until the source closes its channels, signaling that
// there is nothing else to process. Once that happens, and the pipeline has
// been drained (all items have been processed), there are two ways for the
// caller to know: the error channel returned from Go is closed, or the channel
// returned from Done is closed.
//
// The first way can be used if errors need to be processed. A simple loop
// could look like this:
//
//    errs := batch.Go(ctx, s, p)
//    for err := range errs {
//      // Log the error here...
//      log.Print(err.Error())
//    }
//    // Now batch processing is done
//
// If the errors don't need to be processed, the IgnoreErrors function can be
// used to drain the error channel. Then the Done channel can be used to
// determine whether or not batch processing is complete:
//
//    IgnoreErrors(batch.Go(ctx, s, p))
//    <-batch.Done()
//    // Now batch processing is done
//
// Note that the errors returned on the error channel may be wrapped in a
// BatchError so the caller knows whether they come from the source or the
// processor (or neither). Errors from the source will be of type SourceError,
// and errors from the processor will be of type ProcessorError. Errors from
// Batch itself will be neither.
type Batch struct {
	config          BatchConfig
	readConcurrency uint64

	src   source.Source
	proc  processor.Processor
	items chan interface{}
	done  chan struct{}

	// mu protects the following variables. The reason errs is protected is
	// to avoid sending on a closed channel in the Go method.
	mu      sync.Mutex
	running bool
	errs    chan error
}

// New creates a new Batch based on specified config. If config is nil,
// the default config is used as described in Batch.
func New(config BatchConfig, readConcurrency uint64) *Batch {
	return &Batch{
		config:          config,
		readConcurrency: readConcurrency,
	}
}

// Go starts batch processing asynchronously and returns a channel to
// which errors are written. When processing is done and the pipeline
// is drained, the error channel is closed.
//
// Even though Go runs concurrently, concurrent calls to Go are not
// allowed. If Go is called before a previous call completes, the second
// one will panic.
//
//    // NOTE: bad - this will panic!
//    errs := batch.Go(ctx, s, p)
//    errs2 := batch.Go(ctx, s, p) // this call panics
//
// Note that Go does not stop if ctx is done. Otherwise loss of data
// could occur. Suppose the source reads item A and then ctx is canceled.
// If Go were to return right away, item A would not be processed and it
// would be lost forever.
//
// To avoid situations like that, a proper way to handle context completion
// is for the source to check for ctx done and then close its channels. The
// batch processor realizes the source is finished reading items and it sends
// all remaining items to the processor for processing. Once the processor is
// done, it closes its error channel to signal to the batch processor.
// Finally, the batch processor signals to its caller that processing is
// complete and the entire pipeline is drained.
func (b *Batch) Go(ctx context.Context, s source.Source, p processor.Processor) <-chan error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.running {
		panic("Concurrent calls to Batch.Go are not allowed")
		return nil
	}

	if b.readConcurrency == 0 {
		b.readConcurrency = 1
	}

	if b.config == nil {
		b.config = NewConstantBatchConfig(nil)
	}

	b.running = true
	b.errs = make(chan error)

	b.src = s
	b.proc = p
	b.items = make(chan interface{})
	b.done = make(chan struct{})

	go b.doReaders(ctx)
	go b.doProcessors(ctx)

	return b.errs
}

// Done provides an alternative way to determine when processing is
// complete. When it is, the channel is closed, signaling that everything
// is done.
func (b *Batch) Done() <-chan struct{} {
	return b.done
}

func (b *Batch) doReaders(ctx context.Context) {
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	for i := uint64(0); i < b.readConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.read(ctx)
		}()
	}
	wg.Wait()

	close(b.items)
}

func (b *Batch) doProcessors(ctx context.Context) {
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		b.process(ctx)

	}()
	wg.Wait()

	// Once processors are complete, everything is
	b.mu.Lock()
	close(b.errs)
	close(b.done)
	b.running = false
	b.mu.Unlock()
}

func (b *Batch) read(ctx context.Context) {
	items := make(chan interface{})
	errs := make(chan error)

	go b.src.Read(ctx, items, errs)

	var itemsClosed, errsClosed bool
	for !itemsClosed || !errsClosed {
		select {
		case item, ok := <-items:
			if ok {
				b.items <- item
			} else {
				itemsClosed = true
			}
		case err, ok := <-errs:
			if ok {
				b.errs <- newSourceError(err)
			} else {
				errsClosed = true
			}
		}
	}
}

func (b *Batch) process(ctx context.Context) {
	var (
		wg      sync.WaitGroup
		done    bool
		bufSize uint64
	)

	// Process one batch each time
	for !done {
		config := b.config.Get()

		if config.MaxTime > 0 && config.MinTime > 0 && config.MaxTime < config.MinTime {
			config.MinTime = config.MaxTime
		}
		if config.MaxItems > 0 && config.MinItems > 0 && config.MaxItems < config.MinItems {
			config.MinItems = config.MaxItems
		}

		// TODO smarter buffer size (perhaps from the config)
		if config.MaxItems > 0 {
			bufSize = config.MaxItems
		} else if config.MinItems > 0 {
			bufSize = config.MinItems * 2
		} else {
			bufSize = 1024
		}

		var (
			reachedMinTime bool
			itemsRead      uint64

			items = make([]interface{}, 0, bufSize)

			minTimer <-chan time.Time
			maxTimer <-chan time.Time
		)

		// Be careful not to set timers that end right away. Instead, if a
		// min or max time is not specified, make a timer channel that's never
		// written to
		if config.MinTime > 0 {
			minTimer = time.After(config.MinTime)
		} else {
			minTimer = make(chan time.Time)
			reachedMinTime = true
		}

		if config.MaxTime > 0 {
			maxTimer = time.After(config.MaxTime)
		} else {
			maxTimer = make(chan time.Time)
		}

	loop:
		for {
			select {
			case item, ok := <-b.items:
				if ok {
					items = append(items, item)
					itemsRead++
					if itemsRead >= config.MinItems && reachedMinTime {
						break loop
					} else if config.MaxItems > 0 && itemsRead >= config.MaxItems {
						break loop
					}
				} else {
					// Done, break loop and finish processing the
					// remaining items no matter what
					done = true
					break loop
				}

			case <-minTimer:
				reachedMinTime = true
				if itemsRead >= config.MinItems {
					break loop
				}

			case <-maxTimer:
				break loop
			}
		}

		// TODO this resets the time whenever no items are available. The right
		// way to do it is to have the above loop not break until at least one
		// item is available
		if len(items) == 0 {
			continue
		}

		// Process all current items
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs := make(chan error)
			go b.proc.Process(ctx, items, errs)
			for err := range errs {
				b.errs <- newProcessorError(err)
			}
		}()
	}

	// Wait for all processing to complete
	wg.Wait()
}
