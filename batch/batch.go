// Package batch contains the core batch processing functionality. The main
// type is Batch, which can be created using New. It reads from an
// implementation of the Source interface, and items are processed in
// batches by one or more implementations of the Processor interface. Some Source
// and Processor implementations may be provided in related packages,
// or you can create your own based on your needs.
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
//	MaxTime = MaxItems > EOF > MinTime > MinItems
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
// Multiple processors can be chained together when calling Go(). In a processor
// chain, each processor receives the output items from the previous processor,
// allowing for multi-stage processing pipelines:
//
//	// Chain three processors together
//	b.Go(ctx, source, validator, transformer, enricher)
//
// Note that the timers and item counters are relative to the time when the
// previous batch started processing. Just before the timers and counters are
// started the config is read from the Config interface. This is so that
// the configuration can be changed at any time during processing.
package batch

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Batch provides batch processing given a Source and one or more Processors. Data is
// read from the Source and processed in batches through the Processors. Any errors
// are wrapped in either a SourceError or a ProcessorError, so the caller
// can determine where the errors came from.
//
// To create a new Batch, call the New function. Creating one using &Batch{}
// will return the default Batch.
//
//	// The following are equivalent
//	defaultBatch1 := &batch.Batch{}
//	defaultBatch2 := batch.New(nil)
//	defaultBatch3 := batch.New(batch.NewConstantConfig(&batch.ConfigValues{}))
//
// The defaults (with nil Config) provide a usable, but likely suboptimal, Batch
// where items are processed as soon as they are retrieved from the source.
// Processing is done in the background using as many goroutines as necessary.
//
// Batch runs asynchronously until the source is exhausted and all items have been
// processed. There are two ways for the caller to know when processing is complete:
// the error channel returned from Go() is closed, or the channel returned from
// Done() is closed.
//
// The first way can be used if errors need to be processed elsewhere. A simple
// loop could look like this:
//
//	errs := b.Go(ctx, s, p)
//	for err := range errs {
//	  // Log the error here...
//	  log.Print(err.Error())
//	}
//	// Now batch processing is done
//
// If the errors don't need to be processed, the IgnoreErrors function can be
// used to drain the error channel. Then the Done channel can be used to
// determine whether or not batch processing is complete:
//
//	batch.IgnoreErrors(b.Go(ctx, s, p))
//	<-b.Done()
//	// Now batch processing is done
//
// For synchronous processing where you want to wait for completion and collect all errors,
// the RunBatchAndWait helper function provides a convenient way to do this:
//
//	errors := batch.RunBatchAndWait(ctx, b, source, processor)
//	// Process errors here
//	for _, err := range errors {
//		log.Printf("Error: %v", err)
//	}
//
// Note that the errors returned on the error channel may be wrapped in a
// batch.Error so the caller knows whether they come from the source or the
// processor (or neither). Errors from the source will be of type SourceError,
// and errors from the processor will be of type ProcessorError. Errors from
// Batch itself will be neither.
type Batch struct {
	config     Config
	src        Source
	processors []Processor
	items      chan *Item
	ids        chan uint64
	done       chan struct{}

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

// Item represents a single data item being processed through the batch pipeline.
// Each item has a unique ID, the data payload being processed, and an optional
// error field that processors can set when item-specific processing fails.
type Item struct {
	ID    uint64      // Do not modify - unique identifier for tracking items
	Data  interface{} // Safe to modify during processing - contains the item data
	Error error       // Populated by processors to track per-item failure
}

// Source reads items that are to be batch processed.
type Source interface {
	// Read reads items from a data source and returns two channels:
	// - out: for data items
	// - errs: for any errors encountered during reading
	//
	// Each implementation should:
	// 1. Set up appropriate channels for data and errors
	// 2. Start a goroutine to read from the source
	// 3. Close both channels when reading is complete
	// 4. Respect context cancellation
	//
	// Example implementation:
	//
	//	func (s *MySource) Read(ctx context.Context) (out <-chan interface{}, errs <-chan error) {
	//		dataCh := make(chan interface{})
	//		errCh := make(chan error)
	//
	//		go func() {
	//			defer close(dataCh)
	//			defer close(errCh)
	//
	//			// Read items until done...
	//			for _, item := range s.items {
	//				select {
	//				case <-ctx.Done():
	//					errCh <- ctx.Err()
	//					return
	//				case dataCh <- item:
	//					// Item sent successfully
	//				}
	//			}
	//		}()
	//
	//		return dataCh, errCh
	//	}
	//
	// Important: Both channels must be properly closed when reading is finished
	// or when the context is canceled. The source should never leave channels open
	// indefinitely.
	Read(ctx context.Context) (out <-chan interface{}, errs <-chan error)
}

// Processor processes items in batches.
type Processor interface {
	// Process receives a batch of items, performs operations on them, and returns
	// the processed items along with any processor-wide error.
	//
	// Each implementation should:
	// 1. Process items in the batch synchronously
	// 2. Set item.Error on individual items that fail processing
	// 3. Return processor-wide errors separately from per-item errors
	// 4. Respect context cancellation
	//
	// The returned slice may contain a different number of items than the input slice.
	// Items can be added, removed, or reordered as needed.
	//
	// Processors can be chained together in the Go() method, with each processor
	// receiving the output from the previous one:
	//
	//	// Chain three processors together
	//	batch.Go(ctx, source, processor1, processor2, processor3)
	//
	// In a processor chain, each processor receives the output items from the
	// previous processor, allowing for multi-stage processing pipelines. This is
	// useful for separating different processing concerns:
	//
	//	// First processor validates items
	//	// Second processor transforms items
	//	// Third processor enriches items with additional data
	//	batch.Go(ctx, source, validationProc, transformProc, enrichmentProc)
	//
	// Example implementation:
	//
	//	func (p *MyProcessor) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	//		for _, item := range items {
	//			// Skip items that already have errors
	//			if item.Error != nil {
	//				continue
	//			}
	//
	//			// Check for context cancellation
	//			select {
	//			case <-ctx.Done():
	//				return items, ctx.Err()
	//			default:
	//				// Continue processing
	//			}
	//
	//			// Process the item
	//			result, err := p.processItem(item.Data)
	//			if err != nil {
	//				item.Error = err
	//				continue
	//			}
	//
	//			// Update with processed data
	//			item.Data = result
	//		}
	//		return items, nil
	//	}
	//
	// Important:
	// - Never modify the ID field of items
	// - Individual item errors should be set on the item.Error field
	// - Return a non-nil error only for processor-wide failures
	Process(ctx context.Context, items []*Item) ([]*Item, error)
}

// Go starts batch processing asynchronously and returns a channel on
// which errors are written. When processing is done and the pipeline
// is drained, the error channel is closed.
//
// Go launches the entire batch processing pipeline:
// 1. The Source reads items and passes them to the batch collector
// 2. The batch collector groups items according to Config parameters
// 3. Each batch is processed through all Processors in sequence
// 4. Errors from Source and Processors are forwarded to the returned channel
//
// Multiple processors can be chained together in order:
//
//	// Chain three processors in sequence
//	errCh := b.Go(ctx, source, validateProc, transformProc, enrichProc)
//
// Even though Go has several goroutines running concurrently, concurrent
// calls to Go are not allowed. If Go is called before a previous call
// completes, the second one will panic.
//
//	// NOTE: bad - this will panic!
//	errs := b.Go(ctx, s, p)
//	errs2 := b.Go(ctx, s, p) // this call panics
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
//
// Example usage with error handling:
//
//	// Start processing
//	errCh := b.Go(ctx, source, processor)
//
//	// Handle errors as they occur
//	go func() {
//		for err := range errCh {
//			if sourceErr, ok := err.(*batch.SourceError); ok {
//				log.Printf("Source error: %v", sourceErr.Unwrap())
//			} else if procErr, ok := err.(*batch.ProcessorError); ok {
//				log.Printf("Processor error: %v", procErr.Unwrap())
//			} else {
//				log.Printf("Other error: %v", err)
//			}
//		}
//		// Processing is complete when the error channel is closed
//		log.Println("Processing complete")
//	}()
func (b *Batch) Go(ctx context.Context, s Source, procs ...Processor) <-chan error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.running {
		panic("Concurrent calls to Batch.Go are not allowed")
	}

	if b.config == nil {
		b.config = NewConstantConfig(nil)
	}

	b.running = true

	// Check if source is nil and return error if it is
	if s == nil {
		b.errs = make(chan error, 1)
		b.done = make(chan struct{})
		b.errs <- errors.New("source cannot be nil")
		close(b.errs)
		close(b.done)
		b.running = false
		return b.errs
	}

	b.src = s

	// Filter out nil processors
	b.processors = make([]Processor, 0, len(procs))
	for _, p := range procs {
		if p != nil {
			b.processors = append(b.processors, p)
		}
	}

	b.items = make(chan *Item, 100)
	b.ids = make(chan uint64, 100)
	b.errs = make(chan error, 100)
	b.done = make(chan struct{})

	go b.doIDGenerator()
	go b.doReader(ctx)
	go b.doProcessors(ctx)

	return b.errs
}

// Done provides a channel that is closed when processing is complete.
// This can be used to block until batch processing finishes or to be notified
// of completion in a select statement.
//
// Example usage:
//
//	// Start processing and ignore errors
//	batch.IgnoreErrors(b.Go(ctx, source, processor))
//
//	// Wait for completion
//	<-b.Done()
//	fmt.Println("Processing complete")
//
// Or with a select statement:
//
//	select {
//	case <-b.Done():
//		fmt.Println("Processing complete")
//	case <-ctx.Done():
//		fmt.Println("Context canceled")
//	case <-time.After(timeout):
//		fmt.Println("Timed out waiting for completion")
//	}
func (b *Batch) Done() <-chan struct{} {
	return b.done
}

// doIDGenerator generates unique IDs for the items in the pipeline.
// It runs as a goroutine and continuously increments IDs starting from 0,
// sending them on the ids channel. It terminates when the done channel is closed.
//
// Each item processed by the batch system receives a unique ID that allows
// tracking it through the pipeline. This ID should never be modified by
// processors.
func (b *Batch) doIDGenerator() {
	var id uint64
	for {
		select {
		case b.ids <- id:
			id++
		case <-b.done:
			return
		}
	}
}

// doReader reads from the Source and forwards items to the batch processor.
// It runs as a goroutine and continuously reads from the Source's output and
// error channels until both are closed. For each item read, it:
// 1. Obtains a unique ID from the ID generator
// 2. Creates an Item with the ID and data
// 3. Sends the Item to the batch processor
//
// Any errors from the Source are wrapped in a SourceError and forwarded
// to the error channel. When the Source is exhausted (both channels closed),
// doReader closes the items channel to signal completion to the batch processor.
func (b *Batch) doReader(ctx context.Context) {
	// Get channels from source
	out, errs := b.src.Read(ctx)

	// Handle nil channels from source - just report an error and finish
	if out == nil || errs == nil {
		b.errs <- errors.New("invalid source implementation: returned nil channel(s)")
		close(b.items)
		return
	}

	var outClosed, errsClosed bool
	for !outClosed || !errsClosed {
		select {
		case data, ok := <-out:
			if !ok {
				outClosed = true
				continue
			}
			id := <-b.ids
			b.items <- &Item{
				ID:   id,
				Data: data,
			}

		case err, ok := <-errs:
			if !ok {
				errsClosed = true
				continue
			}
			b.errs <- &SourceError{Err: err}
		}
	}

	close(b.items)
}

// doProcessors handles the batch collection and processing of items.
// It runs as a goroutine and performs the following steps:
//  1. Collects items into batches according to configuration parameters
//  2. For each batch, starts a processing goroutine that:
//     a. Applies each processor in sequence to the batch
//     b. Reports processor-wide errors to the error channel
//     c. Reports item-specific errors to the error channel
//  3. When the source is exhausted, waits for all processing to complete
//  4. Signals completion by closing the error and done channels
//
// Multiple batches may be processed concurrently, but each batch is processed
// through the entire processor chain before the next batch starts processing.
// Within a batch, processors are applied sequentially, with each processor
// receiving the output of the previous one.
func (b *Batch) doProcessors(ctx context.Context) {
	var wg sync.WaitGroup

	for {
		config := fixConfig(b.config.Get())
		batch := b.waitForItems(ctx, config)

		// Only exit the loop if we have no items to process
		if len(batch) == 0 {
			break
		}

		wg.Add(1)
		go func(items []*Item) {
			defer wg.Done()
			for _, proc := range b.processors {
				// Skip nil processors (although they should have been filtered out in Go)
				if proc == nil {
					continue
				}

				var err error
				items, err = proc.Process(ctx, items)
				if err != nil {
					b.errs <- &ProcessorError{Err: err}
				}
			}

			for _, item := range items {
				if item.Error != nil {
					b.errs <- &ProcessorError{Err: item.Error}
				}
			}
		}(batch)
	}

	wg.Wait()
	close(b.errs)
	close(b.done)
	b.mu.Lock()
	b.running = false
	b.mu.Unlock()
}

// fixConfig corrects any invalid configuration values to ensure
// consistent batch processing behavior. It applies the following corrections:
//
// 1. If MinItems is 0, it sets it to 1 (at least one item must be processed)
// 2. If MaxTime < MinTime, it sets MinTime = MaxTime
// 3. If MaxItems < MinItems, it sets MinItems = MaxItems
//
// This ensures that batching logic operates correctly even with inconsistent
// configuration values, following the principle of "fail safe" by adjusting
// parameters rather than causing runtime errors.
//
// Example:
//
//	// Invalid config (max < min)
//	config := ConfigValues{
//		MinItems: 100,
//		MaxItems: 50,
//	}
//
//	// After fixConfig, MinItems will be adjusted to match MaxItems
//	fixedConfig := fixConfig(config)
//	// fixedConfig.MinItems == 50
func fixConfig(c ConfigValues) ConfigValues {
	if c.MinItems == 0 {
		c.MinItems = 1
	}
	if c.MaxTime > 0 && c.MinTime > 0 && c.MaxTime < c.MinTime {
		c.MinTime = c.MaxTime
	}
	if c.MaxItems > 0 && c.MinItems > 0 && c.MaxItems < c.MinItems {
		c.MinItems = c.MaxItems
	}
	return c
}

// waitForItems collects items according to the batch configuration and returns
// the collected batch. It implements the batching strategy according to the
// configuration parameters.
//
// The priority order for batch processing decisions is:
//
//	MaxTime = MaxItems > EOF > MinTime > MinItems
//
// This means:
// - If MaxItems is reached, process immediately (even if MinTime hasn't elapsed)
// - If MaxTime is reached, process if at least one item is available
// - If the source is exhausted (EOF), process any remaining items
// - If MinTime has elapsed, process if MinItems is also satisfied
// - If MinItems is reached, wait until MinTime has also elapsed
//
// Examples of different configurations:
//
//  1. Process each item individually:
//     ConfigValues{MinItems: 1, MaxItems: 1}
//
//  2. Process in exact batches of 100:
//     ConfigValues{MinItems: 100, MaxItems: 100}
//
//  3. Process at most every 5 seconds, or when 1000 items are ready:
//     ConfigValues{MaxTime: 5*time.Second, MaxItems: 1000}
//
//  4. Wait for at least 1 second and 100 items, but no more than 5 seconds or 1000 items:
//     ConfigValues{MinTime: 1*time.Second, MinItems: 100, MaxTime: 5*time.Second, MaxItems: 1000}
//
// The method returns a slice containing the collected batch
func (b *Batch) waitForItems(ctx context.Context, config ConfigValues) []*Item {
	var (
		reachedMinTime bool
		batch          = make([]*Item, 0, config.MinItems)
		minTimer       <-chan time.Time
		maxTimer       <-chan time.Time
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
			if !ok {
				// Source is exhausted, return any remaining items regardless of MinItems, it can be empty
				return batch
			}

			batch = append(batch, item)

			if uint64(len(batch)) >= config.MinItems && reachedMinTime {
				return batch
			}
			if config.MaxItems > 0 && uint64(len(batch)) >= config.MaxItems {
				return batch
			}

		case <-minTimer:
			reachedMinTime = true
			if uint64(len(batch)) >= config.MinItems {
				return batch
			}
			// Keep waiting until MinItems is met

		case <-maxTimer:
			if len(batch) > 0 {
				return batch
			}
			// If maxTimer fires with no items, wait for items

		}
	}
}
