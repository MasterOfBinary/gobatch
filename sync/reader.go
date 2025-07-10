package sync

import (
	"context"
	"errors"
	"sync"

	"github.com/MasterOfBinary/gobatch/batch"
)

// BatchReader provides synchronous read operations that are batched behind the scenes.
// It uses generics to provide type safety for keys and values.
type BatchReader[K comparable, V any] struct {
	input  chan *readRequest[K, V]
	batch  *batch.Batch
	closed bool
	mu     sync.Mutex
}

// NewBatchReader creates a new BatchReader with the specified configuration and read function.
// The readFunc will be called with batches of keys to fetch.
func NewBatchReader[K comparable, V any](config batch.Config, readFunc ReadFunc[K, V]) *BatchReader[K, V] {
	input := make(chan *readRequest[K, V], 100)
	src := &readSource[K, V]{input: input}
	proc := &readProcessor[K, V]{readFunc: readFunc}

	b := batch.New(config)
	errs := b.Go(context.Background(), src, proc)
	batch.IgnoreErrors(errs)

	return &BatchReader[K, V]{
		input: input,
		batch: b,
	}
}

// Get retrieves a value by key. It blocks until the batched operation completes
// or the context is cancelled. Multiple concurrent Get calls will be batched
// together according to the batch configuration.
func (r *BatchReader[K, V]) Get(ctx context.Context, key K) (V, error) {
	var zero V

	if ctx == nil {
		ctx = context.Background()
	}

	req := &readRequest[K, V]{
		ctx:      ctx,
		key:      key,
		response: make(chan readResponse[V], 1),
	}

	// Send request
	select {
	case r.input <- req:
	case <-ctx.Done():
		return zero, ctx.Err()
	}

	// Wait for response
	select {
	case resp := <-req.response:
		return resp.value, resp.err
	case <-ctx.Done():
		return zero, ctx.Err()
	}
}

// Close gracefully shuts down the BatchReader and waits for pending operations to complete.
func (r *BatchReader[K, V]) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return
	}

	r.closed = true
	close(r.input)
	<-r.batch.Done()
}

// readSource implements batch.Source for read requests.
type readSource[K comparable, V any] struct {
	input <-chan *readRequest[K, V]
}

func (s *readSource[K, V]) Read(ctx context.Context) (<-chan interface{}, <-chan error) {
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

// readProcessor implements batch.Processor for read requests.
type readProcessor[K comparable, V any] struct {
	readFunc ReadFunc[K, V]
}

func (p *readProcessor[K, V]) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	// Collect active requests
	activeRequests := make([]*readRequest[K, V], 0, len(items))
	keys := make([]K, 0, len(items))

	for _, item := range items {
		req, ok := item.Data.(*readRequest[K, V])
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
			keys = append(keys, req.key)
		}
	}

	if len(keys) == 0 {
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

	// Execute batched read
	values, err := p.readFunc(ctx, keys)
	if err != nil {
		// Global error affects all requests
		for _, req := range activeRequests {
			req.sendError(err)
		}
		return items, err
	}

	// Send individual responses
	for _, req := range activeRequests {
		value, found := values[req.key]
		if !found {
			// Key not found - let the caller decide how to handle
			req.sendResponse(*new(V), errors.New("key not found"))
		} else {
			req.sendResponse(value, nil)
		}
	}

	return items, nil
}
