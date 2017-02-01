package gobatch

import (
	"context"
	"sync"
	"time"

	"github.com/MasterOfBinary/gobatch/processor"
	"github.com/MasterOfBinary/gobatch/source"
)

type batchImpl struct {
	minTime         time.Duration
	minItems        uint64
	maxTime         time.Duration
	maxItems        uint64
	readConcurrency uint64

	running        bool
	doneReaders    bool
	doneProcessors bool

	src  source.Source
	proc processor.Processor

	items chan interface{}
	errs  chan error
	done  <-chan struct{}

	setupOnce sync.Once
	mu        sync.Mutex
}

func (b *batchImpl) Go(ctx context.Context, s source.Source, p processor.Processor) <-chan error {
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.running {
		b.errs <- ErrConcurrentGoCalls
		return b.errs
	}

	b.running = true
	b.items = make(chan interface{})
	b.errs = make(chan error)
	b.src = s
	b.proc = p

	go b.doReaders(ctx)
	go b.doProcessors(ctx)

	return b.errs
}

func (b *batchImpl) doReaders(ctx context.Context) {
	var wg sync.WaitGroup
	for i := 0; i < b.readConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			read(ctx)
		}()
	}
	wg.Wait()

	b.mu.Lock()
	b.doneReaders = true
	b.mu.Unlock()

	b.complete()
}

func (b *batchImpl) doProcessors(ctx context.Context) {
	// ...

	b.mu.Lock()
	b.doneProcessors = true
	b.mu.Unlock()

	b.complete()
}

func (b *batchImpl) read(ctx context.Context) {
	items := make(chan interface{})
	errs := make(chan error)

	go b.src.Read(ctx, items, errs)

	// Read should close the channels when the context is done, so we don't check
	// ctx.Done() here. Otherwise we might return before Read is completely
	// finished. The way we know we've received everything from Read is
	// when the channels have been closed.
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

func (b *batchImpl) complete() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.doneReaders || !b.doneProcessors {
		return
	}

	close(b.done)
	close(b.items)
	close(b.errs)

	b.running = false
}
