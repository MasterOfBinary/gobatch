package sync

import (
	"context"
	"errors"
	"sync"

	"github.com/MasterOfBinary/gobatch/batch"
)

// BatchWriter provides synchronous write operations that are batched behind the scenes.
// It uses generics to provide type safety for keys and values.
type BatchWriter[K comparable, V any] struct {
	input  chan *writeRequest[K, V]
	batch  *batch.Batch
	closed bool
	mu     sync.Mutex
}

// NewBatchWriter creates a new BatchWriter with the specified configuration and write function.
// The writeFunc will be called with batches of key-value pairs to write.
func NewBatchWriter[K comparable, V any](config batch.Config, writeFunc WriteFunc[K, V]) *BatchWriter[K, V] {
	input := make(chan *writeRequest[K, V], 100)
	src := &writeSource[K, V]{input: input}
	proc := &writeProcessor[K, V]{writeFunc: writeFunc}

	b := batch.New(config)
	errs := b.Go(context.Background(), src, proc)
	batch.IgnoreErrors(errs)

	return &BatchWriter[K, V]{
		input: input,
		batch: b,
	}
}

// Set writes a key-value pair. It blocks until the batched operation completes
// or the context is cancelled. Multiple concurrent Set calls will be batched
// together according to the batch configuration.
func (w *BatchWriter[K, V]) Set(ctx context.Context, key K, value V) error {
	if ctx == nil {
		ctx = context.Background()
	}

	req := &writeRequest[K, V]{
		ctx:      ctx,
		key:      key,
		value:    value,
		response: make(chan error, 1),
	}

	// Send request
	select {
	case w.input <- req:
	case <-ctx.Done():
		return ctx.Err()
	}

	// Wait for response
	select {
	case err := <-req.response:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close gracefully shuts down the BatchWriter and waits for pending operations to complete.
func (w *BatchWriter[K, V]) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return
	}

	w.closed = true
	close(w.input)
	<-w.batch.Done()
}

// writeSource implements batch.Source for write requests.
type writeSource[K comparable, V any] struct {
	input <-chan *writeRequest[K, V]
}

func (s *writeSource[K, V]) Read(ctx context.Context) (<-chan interface{}, <-chan error) {
	out := make(chan interface{})
	errs := make(chan error)

	go func() {
		defer close(out)
		defer close(errs)

		for {
			select {
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			case req, ok := <-s.input:
				if !ok {
					return
				}
				out <- req
			}
		}
	}()

	return out, errs
}

// writeProcessor implements batch.Processor for write requests.
type writeProcessor[K comparable, V any] struct {
	writeFunc WriteFunc[K, V]
}

func (p *writeProcessor[K, V]) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	// Collect active requests and build write map
	activeRequests := make([]*writeRequest[K, V], 0, len(items))
	data := make(map[K]V)

	for _, item := range items {
		req, ok := item.Data.(*writeRequest[K, V])
		if !ok {
			item.Error = errors.New("invalid request type")
			continue
		}

		// Check if request context is already cancelled
		select {
		case <-req.getContext().Done():
			req.sendError(req.getContext().Err())
			continue
		default:
			activeRequests = append(activeRequests, req)
			// Last write wins for duplicate keys
			data[req.key] = req.value
		}
	}

	if len(data) == 0 {
		return items, nil
	}

	// Check batch context
	select {
	case <-ctx.Done():
		err := ctx.Err()
		for _, req := range activeRequests {
			req.sendError(err)
		}
		return items, err
	default:
	}

	// Execute batched write
	err := p.writeFunc(ctx, data)

	// Send result to all requests (same error for all in batch)
	for _, req := range activeRequests {
		req.sendError(err)
	}

	return items, err
}
