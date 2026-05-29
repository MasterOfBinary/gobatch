package batch

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// closedDone is a pre-closed channel returned by Done when Go has not been
// called yet. This prevents callers from blocking on a nil channel.
var closedDone = func() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}()

// closedErrs is a pre-closed, empty error channel returned by Go when it rejects
// a call (a nil source or an already-used Batch). Returning a closed channel
// rather than nil keeps a caller that ranges over Go's first return value safe
// even when it ignores the returned error.
var closedErrs = func() chan error {
	ch := make(chan error)
	close(ch)
	return ch
}()

// BufferConfig configures the internal buffer sizes used by Batch.
// If not specified, default values are used.
type BufferConfig struct {
	// ItemBufferSize is the buffer size for the items channel.
	// Default: DefaultItemBufferSize
	ItemBufferSize int

	// ErrorBufferSize is the buffer size for the error channel.
	// Default: DefaultErrorBufferSize
	ErrorBufferSize int
}

// Batch provides batch processing given a Source and one or more Processors.
// Data is read from the Source and processed through each Processor in sequence.
// Any errors are wrapped in either a SourceError or a ProcessorError, so the caller
// can determine where the errors came from.
//
// To create a new Batch, call New. Creating one using &Batch[T]{} will also work.
//
//	// The following are equivalent:
//	defaultBatch1 := &batch.Batch[any]{}
//	defaultBatch2 := batch.New[any](nil)
//	defaultBatch3 := batch.New[any](batch.NewConstantConfig(&batch.ConfigValues{}))
//
// If Config is nil, a default configuration is used, where items are processed
// immediately as they are read.
//
// Batch runs asynchronously after Go is called. When processing is complete,
// either the error channel returned from Go is closed, or the channel returned
// from Done is closed. A Batch is single-use: create a new one with New for
// each run.
//
// A simple way to wait for completion while handling errors:
//
//	errs, err := b.Go(ctx, s, p)
//	if err != nil {
//	  log.Fatal(err)
//	}
//	for err := range errs {
//	  log.Print(err.Error())
//	}
//	// Now batch processing is done
//
// If errors don't need to be handled, IgnoreErrors can be used:
//
//	errs, err := b.Go(ctx, s, p)
//	if err != nil {
//	  log.Fatal(err)
//	}
//	batch.IgnoreErrors(errs)
//	<-b.Done()
//	// Now batch processing is done
//
// Errors returned on the error channel may be wrapped. Source errors will be
// of type SourceError, processor errors will be of type ProcessorError, and
// Batch errors (internal errors) will be plain.
type Batch[T any] struct {
	config       Config
	bufferConfig BufferConfig
	cancelMode   CancelMode
	src          Source[T]
	processors   []Processor[T]
	items        chan *Item[T]
	done         chan struct{}

	mu   sync.Mutex
	used bool
	errs chan error
}

// New creates a new Batch using the provided config. If config is nil,
// a default configuration is used.
//
// To avoid race conditions, the config cannot be changed after the Batch
// is created. Instead, implement the Config interface to support changing
// values.
func New[T any](config Config) *Batch[T] {
	return &Batch[T]{
		config: config,
	}
}

// WithBufferConfig sets custom buffer sizes for the Batch.
// This must be called before Go() is called.
//
// Example:
//
//	b := batch.New[any](config).WithBufferConfig(batch.BufferConfig{
//		ItemBufferSize:  1000,
//		ErrorBufferSize: 500,
//	})
//
// Panics if called after Go() has started to prevent data races and confusion.
func (b *Batch[T]) WithBufferConfig(config BufferConfig) *Batch[T] {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.used {
		panic("batch: WithBufferConfig cannot be called after Go() has started")
	}

	b.bufferConfig = config
	return b
}

// WithCancelMode sets how the Batch reacts to context cancellation.
//
// The default (zero value) is CancelDrain, which keeps processing items
// already read from the Source and relies on the Source to stop producing and
// close its channels. CancelStop instead makes the Batch stop reading promptly
// when the context is canceled; items already buffered in the pipeline are
// still processed, but items not yet read from the Source may be dropped.
//
// Example:
//
//	b := batch.New[any](config).WithCancelMode(batch.CancelStop)
//
// This must be called before Go(). Panics if called after Go() has started to
// prevent data races and confusion.
func (b *Batch[T]) WithCancelMode(m CancelMode) *Batch[T] {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.used {
		panic("batch: WithCancelMode cannot be called after Go() has started")
	}

	b.cancelMode = m
	return b
}

// Item represents a single data item flowing through the batch pipeline.
type Item[T any] struct {
	// ID is a unique identifier for the item. It must not be modified by processors.
	ID uint64

	// Data holds the payload being processed. It is safe for processors to modify.
	Data T

	// Error is set by processors to indicate a failure specific to this item.
	Error error
}

// Source reads items that are to be batch processed.
type Source[T any] interface {
	// Read reads items from a data source and returns two channels:
	// one for items, and one for errors.
	//
	// Read must create both channels (never return nil channels), and must close them
	// when reading is finished or when context is canceled.
	//
	// Example:
	//
	//	func (s *MySource) Read(ctx context.Context) (<-chan int, <-chan error) {
	//		out := make(chan int)
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
	Read(ctx context.Context) (<-chan T, <-chan error)
}

// Processor processes items in batches. Implementations apply operations to each batch
// and may modify items or set per-item errors. Processors can be chained together to
// form multi-stage pipelines.
type Processor[T any] interface {
	// Process applies operations to a batch of items.
	// It may modify item data or set item.Error on individual items.
	//
	// Process should respect context cancellation.
	// It returns the modified slice of items and a processor-wide error, if any.
	//
	// Example:
	//
	//	func (p *MyProcessor) Process(ctx context.Context, items []*batch.Item[int]) ([]*batch.Item[int], error) {
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
	Process(ctx context.Context, items []*Item[T]) ([]*Item[T], error)
}

// Go starts batch processing asynchronously and returns an error channel.
//
// The pipeline consists of the following steps:
//   - Items are read from the Source.
//   - Items are grouped into batches based on the Config.
//   - Each batch is processed through the Processors in sequence.
//
// A Batch is single-use. Go returns the pipeline error channel together with a
// start error:
//   - If the Batch has already been used, Go returns ErrBatchUsed.
//   - If s is nil, Go returns ErrNilSource.
//
// On a start error the returned channel is non-nil and already closed, so it is
// always safe to range over even if the error is not checked. To run again,
// create a new Batch with New. Use errors.Is to test the returned error.
//
// Context cancellation:
//   - The reaction to a canceled context is configurable via WithCancelMode.
//   - The default, CancelDrain, does not immediately stop reading when the
//     context is canceled; it relies on the Source to stop producing and close
//     its channels, and any items already read from the Source are still
//     processed to avoid data loss.
//   - CancelStop instead stops reading promptly on cancellation. Items already
//     buffered in the pipeline are still processed, but items not yet read from
//     the Source may be dropped.
//   - In both modes, internal error sends remain context-aware, so a full,
//     undrained error channel cannot deadlock the pipeline on cancellation.
//
// Example:
//
//	b := batch.New[any](config)
//	errs, err := b.Go(ctx, source, processor)
//	if err != nil {
//		log.Fatal(err)
//	}
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
//   - Items already read into the pipeline are processed even when the context
//     is canceled. Under the default CancelDrain this includes everything the
//     Source eventually produces; under CancelStop it covers only the items
//     buffered before cancellation (see WithCancelMode).
func (b *Batch[T]) Go(ctx context.Context, s Source[T], procs ...Processor[T]) (<-chan error, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// A Batch is single-use. Reject a second call before touching any state so an
	// already-running or already-finished Batch is never disturbed.
	if b.used {
		return closedErrs, ErrBatchUsed
	}

	if s == nil {
		return closedErrs, ErrNilSource
	}

	b.used = true

	if b.config == nil {
		b.config = NewConstantConfig(nil)
	}

	b.src = s

	// Filter out nil processors
	b.processors = make([]Processor[T], 0, len(procs))
	for _, p := range procs {
		if p != nil {
			b.processors = append(b.processors, p)
		}
	}

	// Use custom buffer sizes if specified, otherwise use defaults
	itemBuf := b.bufferConfig.ItemBufferSize
	if itemBuf <= 0 {
		itemBuf = DefaultItemBufferSize
	}
	errBuf := b.bufferConfig.ErrorBufferSize
	if errBuf <= 0 {
		errBuf = DefaultErrorBufferSize
	}

	b.items = make(chan *Item[T], itemBuf)
	b.errs = make(chan error, errBuf)
	b.done = make(chan struct{})

	go b.doReader(ctx)
	go b.doProcessors(ctx)

	return b.errs, nil
}

// Done returns a channel that is closed when batch processing is complete.
//
// The Done channel can be used to wait for processing to finish,
// either by blocking or using a select statement with a timeout or context cancellation.
//
// Example:
//
//	b := batch.New[any](config)
//	errs, err := b.Go(ctx, source, processor)
//	if err != nil {
//		log.Fatal(err)
//	}
//	batch.IgnoreErrors(errs)
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
func (b *Batch[T]) Done() <-chan struct{} {
	// Guard the read of b.done with b.mu: Go assigns b.done while holding the
	// lock, so reading it unlocked is a data race.
	b.mu.Lock()
	done := b.done
	b.mu.Unlock()

	if done == nil {
		return closedDone
	}
	return done
}

// doReader reads items from the Source and forwards them to the batch processor.
//
// It starts the Source.Read goroutine, then listens for data and errors.
// For each data item, it assigns a unique ID and sends it to the items channel.
// For each error, it wraps it in a SourceError and forwards it to the error channel.
//
// When both the data and error channels are closed, it closes the items channel
// to signal that no more data will be produced.
func (b *Batch[T]) doReader(ctx context.Context) {
	// Get channels from source
	out, errs := b.src.Read(ctx)

	// Handle nil channels from source - just report an error and finish.
	// The send to b.errs is context-aware so a cancelled context cannot wedge
	// the reader if the error buffer is full and nobody is draining it.
	if out == nil || errs == nil {
		select {
		case b.errs <- errors.New("invalid source implementation: returned nil channel(s)"):
		case <-ctx.Done():
		}
		close(b.items)
		return
	}

	// nextID is goroutine-local: doReader is the only place item IDs are
	// assigned, and a single-use Batch runs doReader exactly once, so the
	// counter needs no synchronization (no atomic, no lock).
	var nextID uint64

	// stopCh is active only in CancelStop mode. In CancelDrain mode it stays
	// nil, and a receive on a nil channel blocks forever, so the select below
	// behaves exactly as it did before WithCancelMode existed: the reader waits
	// on the Source and relies on it to close its channels. In CancelStop mode
	// stopCh is ctx.Done(), so a canceled context promptly closes b.items and
	// stops reading even if the Source never stops on its own.
	var stopCh <-chan struct{}
	if b.cancelMode == CancelStop {
		stopCh = ctx.Done()
	}
	var outClosed, errsClosed bool
	for !outClosed || !errsClosed {
		select {
		case <-stopCh:
			// CancelStop only: stop reading promptly on cancellation. Items
			// already buffered in b.items are still processed by doProcessors;
			// items not yet read from the Source may be dropped.
			close(b.items)
			return

		case data, ok := <-out:
			if !ok {
				outClosed = true
				out = nil
				continue
			}
			id := nextID
			nextID++
			b.items <- &Item[T]{
				ID:   id,
				Data: data,
			}

		case err, ok := <-errs:
			if !ok {
				errsClosed = true
				errs = nil
				continue
			}
			// Context-aware send: if the error buffer is full and the context
			// is cancelled, stop reading rather than blocking forever.
			select {
			case b.errs <- &SourceError{Err: err}:
			case <-ctx.Done():
				close(b.items)
				return
			}
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
func (b *Batch[T]) doProcessors(ctx context.Context) {
	var wg sync.WaitGroup

	for {
		config := fixConfig(b.config.Get())
		batch := b.waitForItems(ctx, config)

		// Only exit the loop if we have no items to process
		if len(batch) == 0 {
			break
		}

		wg.Add(1)
		go func(items []*Item[T]) {
			defer wg.Done()
			// Recover from panics in user processors so a single buggy
			// Process call cannot crash the host process. Declared after
			// wg.Done so (deferred-LIFO) recover runs first and wg.Done still
			// fires, letting the pipeline complete. The panic is surfaced as a
			// ProcessorError via a context-aware send.
			defer func() {
				if r := recover(); r != nil {
					select {
					case b.errs <- &ProcessorError{Err: fmt.Errorf("processor panic: %v", r)}:
					case <-ctx.Done():
					}
				}
			}()
			for _, proc := range b.processors {
				// Skip nil processors (although they should have been filtered out in Go)
				if proc == nil {
					continue
				}

				var err error
				items, err = proc.Process(ctx, items)
				if err != nil {
					// Context-aware send so a cancelled context can free this
					// goroutine even if the error buffer is full and undrained.
					select {
					case b.errs <- &ProcessorError{Err: err}:
					case <-ctx.Done():
						return
					}
				}
			}

			for _, item := range items {
				if item.Error != nil {
					select {
					case b.errs <- &ProcessorError{Err: item.Error}:
					case <-ctx.Done():
						return
					}
				}
			}
		}(batch)
	}

	wg.Wait()

	// Close done before errs. A CollectErrors caller returns once errs closes,
	// so closing done first guarantees it also observes Done() as closed. No
	// lock is needed: a Batch is single-use, so nothing reassigns done or errs
	// while they are being closed here.
	close(b.done)
	close(b.errs)
}

// maxPreallocCap bounds the capacity that waitForItems will pre-allocate for a
// batch slice. Without this bound, a very large MinItems (reachable, for
// example, via DynamicConfig.UpdateBatchSize(huge, 0)) would be passed directly
// to make, triggering a multi-terabyte allocation that crashes the process.
// The slice still grows as needed via append; this only caps the initial hint.
const maxPreallocCap = 4096

// clampPreallocCap returns a sane, bounded capacity to use when pre-allocating
// a batch slice for the given config values. It never returns more than
// maxPreallocCap, and if maxItems is set it is treated as a hard upper bound on
// the batch size (so the pre-allocation is not larger than the batch can grow).
//
// This guards against pathological configurations where minItems is enormous
// while maxItems is unset, which would otherwise attempt an unbounded
// allocation.
func clampPreallocCap(minItems, maxItems uint64) int {
	c := minItems
	// If a maximum batch size is set, never pre-allocate beyond it.
	if maxItems > 0 && maxItems < c {
		c = maxItems
	}
	if c > maxPreallocCap {
		c = maxPreallocCap
	}
	return int(c)
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
func (b *Batch[T]) waitForItems(_ context.Context, config ConfigValues) []*Item[T] {
	var (
		reachedMinTime bool
		batch          = make([]*Item[T], 0, clampPreallocCap(config.MinItems, config.MaxItems))
		minTimerCh     <-chan time.Time
		maxTimerCh     <-chan time.Time
		minTimer       *time.Timer
		maxTimer       *time.Timer
	)

	// Be careful not to set timers that end right away. Instead, if a
	// min or max time is not specified, leave the channel nil so the
	// select statement ignores it. Timers are stopped on return so a
	// timer does not leak when a batch returns before its timer fires.
	if config.MinTime > 0 {
		minTimer = time.NewTimer(config.MinTime)
		defer minTimer.Stop()
		minTimerCh = minTimer.C
	} else {
		reachedMinTime = true
	}

	if config.MaxTime > 0 {
		maxTimer = time.NewTimer(config.MaxTime)
		defer maxTimer.Stop()
		maxTimerCh = maxTimer.C
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

		case <-minTimerCh:
			reachedMinTime = true
			if uint64(len(batch)) >= config.MinItems {
				return batch
			}
			// Keep waiting until MinItems is met

		case <-maxTimerCh:
			if len(batch) > 0 {
				return batch
			}
			// If max timer fires with no items, restart it so we don't wait indefinitely
			if config.MaxTime > 0 {
				maxTimer.Reset(config.MaxTime)
			}
		}
	}
}
