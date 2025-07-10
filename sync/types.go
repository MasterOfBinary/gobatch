package sync

import (
	"context"
)

// ReadFunc is a user-provided function that performs a batched read operation.
// It receives a slice of keys to fetch and returns a map of results.
// Missing keys can be omitted from the result map or included with zero values.
type ReadFunc[K comparable, V any] func(ctx context.Context, keys []K) (map[K]V, error)

// WriteFunc is a user-provided function that performs a batched write operation.
// It receives a map of key-value pairs to write and returns an error if the
// entire batch fails. For partial failures, implementations should still return
// nil and handle failures internally.
type WriteFunc[K comparable, V any] func(ctx context.Context, data map[K]V) error

// request is the base interface for all sync requests.
type request interface {
	getContext() context.Context
	sendError(err error)
}

// readRequest represents a single read operation in the batch.
type readRequest[K comparable, V any] struct {
	ctx      context.Context
	key      K
	response chan readResponse[V]
}

// readResponse contains the result of a read operation.
type readResponse[V any] struct {
	value V
	err   error
}

func (r *readRequest[K, V]) getContext() context.Context {
	return r.ctx
}

func (r *readRequest[K, V]) sendError(err error) {
	select {
	case r.response <- readResponse[V]{err: err}:
	default:
		// Response channel might be closed if context was cancelled
	}
}

func (r *readRequest[K, V]) sendResponse(value V, err error) {
	select {
	case r.response <- readResponse[V]{value: value, err: err}:
	default:
		// Response channel might be closed if context was cancelled
	}
}

// writeRequest represents a single write operation in the batch.
type writeRequest[K comparable, V any] struct {
	ctx      context.Context
	key      K
	value    V
	response chan error
}

func (w *writeRequest[K, V]) getContext() context.Context {
	return w.ctx
}

func (w *writeRequest[K, V]) sendError(err error) {
	select {
	case w.response <- err:
	default:
		// Response channel might be closed if context was cancelled
	}
}
