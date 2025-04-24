package batch

import (
	"context"
	"sync"
	"time"
)

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

func New(config Config) *Batch {
	return &Batch{
		config: config,
	}
}

type Item struct {
	ID    uint64      // Do not modify
	Data  interface{} // Safe to modify during processing
	Error error       // Populated by processors to track per-item failure
}

// Source provides raw data items to the batch pipeline.
type Source interface {
	Read(ctx context.Context) (out <-chan interface{}, errs <-chan error)
}

// Processor processes batches of items and returns the modified batch or an error.
type Processor interface {
	Process(ctx context.Context, items []*Item) ([]*Item, error)
}

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

	b.src = s
	b.processors = procs
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
func (b *Batch) Done() <-chan struct{} {
	return b.done
}

// Wait blocks until processing is done and collects all errors.
func (b *Batch) Wait() []error {
	var collected []error
	for err := range b.errs {
		collected = append(collected, err)
	}
	<-b.done
	return collected
}

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

// doReader starts the reader goroutine and reads from its channels.
func (b *Batch) doReader(ctx context.Context) {
	out, errs := b.src.Read(ctx)

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

func (b *Batch) doProcessors(ctx context.Context) {
	var wg sync.WaitGroup

	for {
		config := fixConfig(b.config.Get())
		batch, done := b.waitForItems(ctx, config)

		if done || len(batch) == 0 {
			break
		}

		wg.Add(1)
		go func(items []*Item) {
			defer wg.Done()
			for _, proc := range b.processors {
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

func (b *Batch) waitForItems(ctx context.Context, config ConfigValues) ([]*Item, bool) {
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
				return batch, true // no more items
			}

			batch = append(batch, item)

			if uint64(len(batch)) >= config.MinItems && reachedMinTime {
				return batch, false
			}
			if config.MaxItems > 0 && uint64(len(batch)) >= config.MaxItems {
				return batch, false
			}

		case <-minTimer:
			reachedMinTime = true
			if uint64(len(batch)) >= config.MinItems {
				return batch, false
			}
			// Keep waiting until MinItems is met

		case <-maxTimer:
			if len(batch) > 0 {
				return batch, false
			}
			// If maxTimer fires with no items, wait for items

		}
	}
}
