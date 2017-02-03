package gobatch

import (
	"context"
	"sync"
	"time"

	"github.com/MasterOfBinary/gobatch/processor"
	"github.com/MasterOfBinary/gobatch/source"
)

type Batch interface {
	Go(ctx context.Context, s source.Source, p processor.Processor) <-chan error
}

func Must(b Batch, err error) Batch {
	if err != nil {
		panic(err)
	}
	return b
}

type batchImpl struct {
	minTime         time.Duration
	minItems        uint64
	maxTime         time.Duration
	maxItems        uint64
	readConcurrency uint64

	src   source.Source
	proc  processor.Processor
	items chan interface{}

	// mu protects the following variables. The reason errs is protected is
	// to avoid sending on a closed channel in Go.
	mu      sync.Mutex
	running bool
	errs    chan error
}

func (b *batchImpl) Go(ctx context.Context, s source.Source, p processor.Processor) <-chan error {
	b.mu.Lock()

	if b.running {
		defer b.mu.Unlock()
		b.errs <- ErrConcurrentGoCalls
		return b.errs
	}

	b.running = true
	b.errs = make(chan error)
	b.mu.Unlock()

	b.src = s
	b.proc = p
	b.items = make(chan interface{})

	go b.doReaders(ctx)
	go b.doProcessors(ctx)

	b.mu.Lock()
	defer b.mu.Unlock()
	return b.errs
}

func (b *batchImpl) doReaders(ctx context.Context) {
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

func (b *batchImpl) doProcessors(ctx context.Context) {
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
	b.running = false
	b.mu.Unlock()
}

func (b *batchImpl) read(ctx context.Context) {
	items := make(chan interface{})
	errs := make(chan error)

	go b.src.Read(ctx, items, errs)

	var itemsClosed, errsClosed bool
	for {
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
		if itemsClosed && errsClosed {
			break
		}
	}
}

func (b *batchImpl) process(ctx context.Context) {
	var (
		wg   sync.WaitGroup
		done bool
	)

	// Loop, processing one batch each time
	for {
		if done {
			break
		}

		itemsRead := uint64(0)
		items := make([]interface{}, 0, b.maxItems)

	loop:
		for {
			select {
			case item, ok := <-b.items:
				if ok {
					items = append(items, item)
					itemsRead++
					if itemsRead >= b.minItems {
						break loop
					}
				} else {
					// Done, break loop and finish processing the
					// remaining items
					done = true
					break loop
				}
			}
		}

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

	wg.Wait()
}
