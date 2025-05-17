package batch

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Batch provides batch processing given a Source and one or more Processors.
// Data is read from the Source and processed through each Processor in sequence.
// Any errors are wrapped in either a SourceError or a ProcessorError, so the caller
// can determine where the errors came from.
//
// To create a new Batch, call New. Creating one using &Batch{} will also work.
//
//	// The following are equivalent:
//	defaultBatch1 := &batch.Batch{}
//	defaultBatch2 := batch.New(nil)
//	defaultBatch3 := batch.New(batch.NewConstantConfig(&batch.ConfigValues{}))
//
// If Config is nil, a default configuration is used, where items are processed
// immediately as they are read.
//
// Batch runs asynchronously after Go is called. When processing is complete,
// either the error channel returned from Go is closed, or the channel returned
// from Done is closed.
//
// A simple way to wait for completion while handling errors:
//
//	errs := b.Go(ctx, s, p)
//	for err := range errs {
//	  log.Print(err.Error())
//	}
//	// Now batch processing is done
//
// If errors don't need to be handled, IgnoreErrors can be used:
//
//	batch.IgnoreErrors(b.Go(ctx, s, p))
//	<-b.Done()
//	// Now batch processing is done
//
// Errors returned on the error channel may be wrapped. Source errors will be
// of type SourceError, processor errors will be of type ProcessorError, and
// Batch errors (internal errors) will be plain.
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

// New creates a new Batch using the provided config. If config is nil,
// a default configuration is used.
//
// To avoid race conditions, the config cannot be changed after the Batch
// is created. Instead, implement the Config interface to support changing
// values.
func New(config Config) *Batch {
	return &Batch{
		config: config,
	}
}

// Item represents a single data item flowing through the batch pipeline.
type Item struct {
	// ID is a unique identifier for the item. It must not be modified by processors.
	ID uint64

	// Data holds the payload being processed. It is safe for processors to modify.
	Data interface{}

	// Error is set by processors to indicate a failure specific to this item.
	Error error
}

// Source reads items that are to be batch processed.
type Source interface {
	// Read reads items from a data source and returns two channels:
	// one for items, and one for errors.
	//
	// Read must create both channels (never return nil channels), and must close them
	// when reading is finished or when context is canceled.
	//
	// Example:
	//
	//	func (s *MySource) Read(ctx context.Context) (<-chan interface{}, <-chan error) {
	//		out := make(chan interface{})
	//		errs := make(chan error)
	//
	//		go func() {
	//			defer close(out)
	//			defer close(errs)
	//
	//			for _, item := range s.items {
	//				select {
	//				case <-ctx.Done():
	//					errs <- ctx.Err()
	//					return
	//				case out <- item:
	//					// sent successfully
	//				}
	//			}
	//		}()
	//
	//		return out, errs
	//	}
	Read(ctx context.Context) (<-chan interface{}, <-chan error)
}

// Processor processes items in batches. Implementations apply operations to each batch
// and may modify items or set per-item errors. Processors can be chained together to
// form multi-stage pipelines.
type Processor interface {
	// Process applies operations to a batch of items.
	// It may modify item data or set item.Error on individual items.
	//
	// Process should respect context cancellation.
	// It returns the modified slice of items and a processor-wide error, if any.
	//
	// Example:
	//
	//	func (p *MyProcessor) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	//		for _, item := range items {
	//			if item.Error != nil {
	//				continue
	//			}
	//
	//			select {
	//			case <-ctx.Done():
	//				return items, ctx.Err()
	//			default:
	//			}
	//
	//			result, err := p.processItem(item.Data)
	//			if err != nil {
	//				item.Error = err
	//				continue
	//			}
	//
	//			item.Data = result
	//		}
	//
	//		return items, nil
	//	}
	Process(ctx context.Context, items []*Item) ([]*Item, error)
}

// Go starts batch processing asynchronously and returns an error channel.
//
// The pipeline consists of the following steps:
//   - Items are read from the Source.
//   - Items are grouped into batches based on the Config.
//   - Each batch is processed through the Processors in sequence.
//
// Go must only be called once at a time. Calling Go again while a batch is
// already running will cause a panic.
//
// Context cancellation:
//   - Go does not immediately stop processing when the context is canceled.
//   - Any items already read from the Source are still processed to avoid data loss.
//
// Example:
//
//	b := batch.New(config)
//	errs := b.Go(ctx, source, processor)
//
//	go func() {
//		for err := range errs {
//			log.Println("error:", err)
//		}
//	}()
//
//	<-b.Done()
//
// Important:
//   - The Source must close its channels when reading is complete.
//   - Processors must check for context cancellation and stop early if needed.
//   - All items that have already been read will be processed even if the context is canceled.
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

// Done returns a channel that is closed when batch processing is complete.
//
// The Done channel can be used to wait for processing to finish,
// either by blocking or using a select statement with a timeout or context cancellation.
//
// Example:
//
//	b := batch.New(config)
//	batch.IgnoreErrors(b.Go(ctx, source, processor))
//
//	<-b.Done()
//	fmt.Println("Processing complete")
//
// Or using a select statement:
//
//	select {
//	case <-b.Done():
//		fmt.Println("Processing complete")
//	case <-ctx.Done():
//		fmt.Println("Context canceled")
//	case <-time.After(10 * time.Second):
//		fmt.Println("Timed out waiting for processing to finish")
//	}
func (b *Batch) Done() <-chan struct{} {
	return b.done
}

// doIDGenerator generates unique IDs for items in the pipeline.
//
// It runs as a background goroutine, incrementing a counter starting from zero
// and sending each ID on the ids channel. It exits when the done channel is closed.
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

// doReader reads items from the Source and forwards them to the batch processor.
//
// It starts the Source.Read goroutine, then listens for data and errors.
// For each data item, it assigns a unique ID and sends it to the items channel.
// For each error, it wraps it in a SourceError and forwards it to the error channel.
//
// When both the data and error channels are closed, it closes the items channel
// to signal that no more data will be produced.
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

// doProcessors collects items into batches and processes them through the Processor chain.
//
// It runs as a background goroutine and does the following:
//   - Waits for enough items to form a batch based on the current Config.
//   - Starts a goroutine to process each batch through all Processors in sequence.
//   - For each batch, sends any processor-wide errors or item-specific errors to the error channel.
//   - Waits for all batch processing to complete after the source is exhausted.
//   - Signals overall completion by closing the error and done channels.
//
// Batches are processed concurrently, but each batch is processed sequentially through the chain
// of Processors. Each Processor receives the output from the previous one.
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

// fixConfig corrects invalid ConfigValues to ensure consistent batch behavior.
//
// It applies the following adjustments:
//   - If MinItems is zero, it sets it to 1 (at least one item must be processed).
//   - If MaxTime is set and smaller than MinTime, MinTime is reduced to MaxTime.
//   - If MaxItems is set and smaller than MinItems, MinItems is reduced to MaxItems.
//
// These adjustments guarantee that batching rules do not conflict at runtime.
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

// waitForItems collects items from the input channel until a batch is ready.
//
// It implements the batching strategy according to the current ConfigValues, following the priority:
//
//	MaxTime = MaxItems > EOF > MinTime > MinItems
//
// It waits for:
//   - MaxItems: If reached, the batch is processed immediately.
//   - MaxTime: If elapsed and there are items, the batch is processed.
//   - EOF (input closed): Any remaining items are processed.
//   - MinTime: If elapsed and MinItems is satisfied, the batch is processed.
//   - MinItems: If reached, waits until MinTime is also satisfied.
//
// The method returns the collected batch of items.
func (b *Batch) waitForItems(_ context.Context, config ConfigValues) []*Item {
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
				// Source is exhausted, return whatever was collected
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
			// If max timer fires with no items, continue waiting
		}
	}
}
