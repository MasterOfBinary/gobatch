// Package batch contains the core batch processing functionality. The main
// class is Batch, which can be created using New. It reads from an
// implementation of the Source interface, and items are processed in
// batches by an implementation of the Processor interface. Some Source
// and Processor implementations are provided in the source and processor
// packages, respectively, or you can create your own based on your needs.
//
// Batch uses the MinTime, MinItems, MaxTime, and MaxItems configuration
// parameters in Config to determine when and how many items are
// processed at once.
//
// These parameters may conflict, however; for example, during a slow time,
// MaxTime may be reached before MinItems are read. Thus it is necessary
// to prioritize the parameters in some way. They are prioritized as follows
// (with EOF signifying the end of the input data):
//
//    MaxTime = MaxItems > EOF > MinTime > MinItems
//
// A few examples:
//
// MinTime = 2s. After 1s the input channel is closed. The items are
// processed right away.
//
// MinItems = 10, MinTime = 2s. After 1s, 10 items have been read. They are
// not processed until 2s has passed (along with all other items that have
// been read up to the 2s mark).
//
// MaxItems = 10, MinTime = 2s. After 1s, 10 items have been read. They aren't
// processed until 2s has passed.
//
// Note that the timers and item counters are relative to the time when the
// previous batch started processing. Just before the timers and counters are
// started the config is read from the Config interface. This is so that
// the configuration can be changed at any time during processing.
package batch

import (
	"context"
	"sync"
	"time"
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
//    defaultBatch1 := &batch.Batch{}
//    defaultBatch2 := batch.New(nil)
//    defaultBatch3 := batch.New(batch.NewConstantConfig(&batch.ConfigValues{}))
//
// The defaults (with nil Config) provide a usable, but likely suboptimal, Batch
// where items are processed as soon as they are retrieved from the source.
// Processing is done in the background using as many goroutines as necessary.
//
// Both Source and Processor are given a PipelineSource, which contains
// channels for input and output, as well as an error channel. Items in the
// channel are wrapped in an Item struct that contains extra metadata used
// by Batch. For easier usage, the helper function NextItem can be used to
// read from the input channel, set the data, and return the modified Item:
//
//    ps.Output() <- batch.NextItem(ps, item)
//
// Batch runs asynchronously until the source closes its PipelineSource, signaling
// that there is nothing else to read. Once that happens, and the pipeline has
// been drained (all items have been processed), there are two ways for the
// caller to know: the error channel returned from Go is closed, or the channel
// returned from Done is closed.
//
// The first way can be used if errors need to be processed elsewhere. A simple
// loop could look like this:
//
//    errs := myBatch.Go(ctx, s, p)
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
//    batch.IgnoreErrors(myBatch.Go(ctx, s, p))
//    <-myBatch.Done()
//    // Now batch processing is done
//
// Note that the errors returned on the error channel may be wrapped in a
// batch.Error so the caller knows whether they come from the source or the
// processor (or neither). Errors from the source will be of type SourceError,
// and errors from the processor will be of type ProcessorError. Errors from
// Batch itself will be neither.
type Batch struct {
	config Config

	src   Source
	proc  Processor
	items chan *Item
	ids   chan uint64 // For unique IDs
	done  chan struct{}

	// mu protects the following variables. The reason errs is protected is
	// to avoid sending on a closed channel in the Go method.
	mu      sync.Mutex
	running bool
	errs    chan error
}

// New creates a new Batch based on specified config. If config is nil,
// the default config is used as described in Batch.
//
// To avoid race conditions, the config cannot be changed after the Batch
// is created. Instead, implement the Config interface to support changing
// values.
func New(config Config) *Batch {
	return &Batch{
		config: config,
	}
}

// Source reads items that are to be batch processed.
type Source interface {
	// Read reads items from somewhere and writes them to the Output
	// channel of ps. Any errors it encounters while reading are written to the
	// Errors channel. The Input channel provides a steady stream of Items that
	// have pre-set metadata so the batch processor can identify them. A helper
	// function, NextItem, can be used to retrieve an item from the channel,
	// set it, and return it:
	//
	//    items <- batch.NextItem(ps, myData)
	//
	// Read is only run in a single goroutine. Any currency must be provided
	// by the implementation.
	//
	// Once reading is finished (or when the program ends), the batch
	// processor needs to be notified. This is done by calling the Close
	// method on ps, which signals to Batch that it should drain the pipeline
	// and finish. It is not enough for Read to return.
	//
	//    func (s source) Read(ctx context.Context, ps *batch.PipelineStage) {
	//      defer ps.Close()
	//      // Read items until done...
	//    }
	//
	// Read should not modify an item after adding it to items.
	Read(ctx context.Context, ps *PipelineStage)
}

// Processor processes items in batches.
type Processor interface {
	// Process processes items from ps's Input channel and returns any errors
	// encountered on the Errors channel. When it is done, it must close ps
	// to signify that it's finished processing. Simply returning isn't enough.
	//
	//    func (p *processor) Process(ctx context.Context, ps *batch.PipelineStage) {
	//      defer ps.Close()
	//      // Do processing here...
	//    }
	//
	// Batch does not wait for Process to finish, so it can spawn a
	// goroutine and then return, as long as ps is closed at the end.
	//
	//    // This is ok
	//    func (p *processor) Process(ctx context.Context, ps *batch.PipelineStage) {
	//      go func() {
	//        defer ps.Close()
	//        time.Sleep(time.Second)
	//        fmt.Println(items)
	//      }()
	//    }
	//
	// To allow Processors to be chained together, processed items should
	// be returned on the Output channel:
	//
	//    // Process squares values in batches.
	//    func (p *processor) Process(ctx context.Context, ps *batch.PipelineStage) {
	//      defer ps.Close()
	//      for item := range ps.Input() {
	//        value, _ := item.Get().(int64)
	//        item.Set(value*value)
	//        ps.Output() <- item
	//      }
	//    }
	//
	// Process may be run in any number of concurrent goroutines. If
	// concurrency needs to be limited it must be done in Process; for
	// example, by using a semaphore channel.
	Process(ctx context.Context, ps *PipelineStage)
}

// Go starts batch processing asynchronously and returns a channel on
// which errors are written. When processing is done and the pipeline
// is drained, the error channel is closed.
//
// Even though Go has several goroutines running concurrently, concurrent
// calls to Go are not allowed. If Go is called before a previous call
// completes, the second one will panic.
//
//    // NOTE: bad - this will panic!
//    errs := batch.Go(ctx, s, p)
//    errs2 := batch.Go(ctx, s, p) // this call panics
//
// Note that Go does not stop if ctx is done. Otherwise loss of data could occur.
// Suppose the source reads item A and then ctx is canceled. If Go were to return
// right away, item A would not be processed and it would be lost.
//
// To avoid situations like that, a proper way to handle context completion
// is for the source to check for ctx done and then close its channels. The
// batch processor realizes the source is finished reading items and it sends
// all remaining items to the processor for processing. Once the processor is
// done, it closes its error channel to signal to the batch processor.
// Finally, the batch processor signals to its caller that processing is
// complete and the entire pipeline is drained.
func (b *Batch) Go(ctx context.Context, s Source, p Processor) <-chan error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.running {
		panic("Concurrent calls to Batch.Go are not allowed")
	}

	if b.config == nil {
		b.config = NewConstantConfig(nil)
	}

	b.running = true
	b.errs = make(chan error)

	b.src = s
	b.proc = p
	b.items = make(chan *Item)
	b.ids = make(chan uint64)
	b.done = make(chan struct{})

	go b.doIDGenerator()
	go b.doReader(ctx)
	go b.doProcessors(ctx)

	return b.errs
}

// Done provides an alternative way to determine when processing is
// complete. When it is, the channel is closed, signaling that everything
// is done.
func (b *Batch) Done() <-chan struct{} {
	return b.done
}

// doIDGenerator generates unique IDs for the items in the pipeline.
func (b *Batch) doIDGenerator() {
	for id := uint64(0); ; id++ {
		select {
		case b.ids <- id:

		case <-b.done:
			return
		}
	}
}

// doReader starts the reader goroutine and reads from its channels.
func (b *Batch) doReader(ctx context.Context) {
	in := make(chan *Item)
	out := make(chan *Item)
	errs := make(chan error)
	ps := &PipelineStage{
		Input:  in,
		Output: out,
		Errors: errs,
	}

	go b.src.Read(ctx, ps)

	nextItem := &Item{
		id: <-b.ids,
	}

	var outClosed, errClosed bool
	for !outClosed || !errClosed {
		select {
		case in <- nextItem:
			nextItem = &Item{
				id: <-b.ids,
			}

		case item, ok := <-out:
			if ok {
				b.items <- item
			} else {
				outClosed = true
			}

		case err, ok := <-errs:
			if ok {
				b.errs <- &SourceError{
					err: err,
				}
			} else {
				errClosed = true
			}
		}
	}

	close(b.items)
}

// doProcessors starts the processor goroutine.
func (b *Batch) doProcessors(ctx context.Context) {
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

func fixConfig(c ConfigValues) ConfigValues {
	if c.MaxTime > 0 && c.MinTime > 0 && c.MaxTime < c.MinTime {
		c.MinTime = c.MaxTime
	}
	if c.MaxItems > 0 && c.MinItems > 0 && c.MaxItems < c.MinItems {
		c.MinItems = c.MaxItems
	}
	return c
}

func (b *Batch) process(ctx context.Context) {
	var (
		wg      sync.WaitGroup
		done    bool
		bufSize uint64
	)

	// Process one batch each time
	for !done {
		config := fixConfig(b.config.Get())

		// TODO smarter buffer size (perhaps from the config)
		if config.MaxItems > 0 {
			bufSize = config.MaxItems
		} else if config.MinItems > 0 {
			bufSize = config.MinItems * 2
		} else {
			bufSize = 1024
		}

		var items = make([]*Item, 0, bufSize)
		done, items = b.waitForItems(ctx, items, &config)

		// TODO this resets the time whenever no items are available. Need to
		// decide if that's the right way to do it.
		if len(items) == 0 {
			continue
		}

		// Process all current items
		wg.Add(1)
		go func() {
			defer wg.Done()

			in := make(chan *Item)
			out := make(chan *Item)
			errs := make(chan error)
			ps := &PipelineStage{
				Input:  in,
				Output: out,
				Errors: errs,
			}

			go b.proc.Process(ctx, ps)

			go func() {
				for _, item := range items {
					in <- item
				}
				close(in)
			}()

			var outClosed, errClosed bool
			for !outClosed || !errClosed {
				select {
				case item, ok := <-out:
					if ok {
						b.items <- item
					} else {
						outClosed = true
					}

				case err, ok := <-errs:
					if ok {
						b.errs <- &ProcessorError{
							err: err,
						}
					} else {
						errClosed = true
					}
				}
			}
		}()
		wg.Wait()
	}

	// Wait for all processing to complete
	wg.Wait()
}

// waitForItems waits until enough items are read to begin batch processing, based
// on config. It returns true if processing is completely finished, and false
// otherwise.
func (b *Batch) waitForItems(ctx context.Context, items []*Item, config *ConfigValues) (bool, []*Item) {
	var (
		reachedMinTime bool
		itemsRead      uint64

		minTimer <-chan time.Time
		maxTimer <-chan time.Time
	)

	// Be careful not to set timers that end right away. Instead, if a
	// min or max time is not specified, make a timer channel that's never
	// written to.
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

	for {
		select {
		case item, ok := <-b.items:
			if ok {
				items = append(items, item)
				itemsRead++
				if itemsRead >= config.MinItems && reachedMinTime {
					return false, items
				}
				if config.MaxItems > 0 && itemsRead >= config.MaxItems {
					return false, items
				}
			} else {
				// Finished processing
				return true, items
			}

		case <-minTimer:
			reachedMinTime = true
			if itemsRead >= config.MinItems {
				return false, items
			}

		case <-maxTimer:
			return false, items
		}
	}
}
