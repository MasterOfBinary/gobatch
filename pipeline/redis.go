// Package pipeline contains implementations that combine both Source and Processor functionality.
// This enables higher-level abstractions where a single component can handle both data sourcing
// and processing, providing simplified APIs for common batch processing tasks.
package pipeline

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
)

// RedisPipeline combines a Source and Processor to provide a synchronous API
// for batched Redis operations. It allows callers to make simple calls while
// batching happens transparently in the background.
//
// Example usage:
//
//	redisPipeline := pipeline.NewRedisPipeline(redisClient, options)
//	value, err := redisPipeline.Get(ctx, "my-key")
type RedisPipeline struct {
	// redisClient is the Redis client to use for operations.
	// This would typically be something like *redis.Client from go-redis/redis.
	redisClient interface{}

	// batchProcessor is the underlying batch processor that handles batching.
	batchProcessor *batch.Batch

	// Tracks whether the pipeline has been started.
	started bool
	// Guards access to started flag and allows safe concurrent calls.
	mu sync.Mutex

	// requestCh is used to submit requests to the pipeline.
	requestCh chan *redisRequest

	// Used to signal shutdown of the batch processing.
	shutdown     chan struct{}
	shutdownDone chan struct{}
}

// RedisOperation represents the type of Redis operation to perform.
type RedisOperation string

const (
	// Get retrieves a value for a key.
	Get RedisOperation = "GET"
	// Set stores a key-value pair.
	Set RedisOperation = "SET"
	// Del deletes a key.
	Del RedisOperation = "DEL"
	// Exists checks if a key exists.
	Exists RedisOperation = "EXISTS"
)

// redisRequest represents a request to perform a Redis operation.
type redisRequest struct {
	// op is the Redis operation to perform.
	op RedisOperation
	// key is the Redis key to operate on.
	key string
	// value is the optional value for operations like SET.
	value interface{}
	// respCh is the channel to send the response on.
	respCh chan *redisResponse
}

// redisResponse represents the response from a Redis operation.
type redisResponse struct {
	// value is the result of the operation.
	value interface{}
	// err is any error that occurred.
	err error
}

// RedisPipelineOptions provides configuration options for the RedisPipeline.
type RedisPipelineOptions struct {
	// MinItems is the minimum number of items to process in a batch.
	MinItems uint64
	// MaxItems is the maximum number of items to process in a batch.
	MaxItems uint64
	// MinTime is the minimum time to wait before processing a batch.
	MinTime time.Duration
	// MaxTime is the maximum time to wait before processing a batch.
	MaxTime time.Duration
	// BufferSize is the size of the internal request channel buffer.
	BufferSize int
}

// DefaultRedisPipelineOptions returns sensible default options for a RedisPipeline.
func DefaultRedisPipelineOptions() *RedisPipelineOptions {
	return &RedisPipelineOptions{
		MinItems:   10,
		MaxItems:   100,
		MinTime:    5 * time.Millisecond,
		MaxTime:    50 * time.Millisecond,
		BufferSize: 1000,
	}
}

// NewRedisPipeline creates a new RedisPipeline with the given Redis client and options.
func NewRedisPipeline(redisClient interface{}, options *RedisPipelineOptions) *RedisPipeline {
	if options == nil {
		options = DefaultRedisPipelineOptions()
	}

	// Create config for the batch processor
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: options.MinItems,
		MaxItems: options.MaxItems,
		MinTime:  options.MinTime,
		MaxTime:  options.MaxTime,
	})

	// Create the batch processor
	batchProcessor := batch.New(config)

	return &RedisPipeline{
		redisClient:    redisClient,
		batchProcessor: batchProcessor,
		requestCh:      make(chan *redisRequest, options.BufferSize),
		shutdown:       make(chan struct{}),
		shutdownDone:   make(chan struct{}),
	}
}

// Start begins the background batch processing. This must be called before
// any operations can be performed.
func (p *RedisPipeline) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.started {
		return errors.New("pipeline already started")
	}

	p.started = true

	// Start the batch processor with our custom source and processor
	errs := p.batchProcessor.Go(ctx, p, p)

	// Handle errors from the batch processor
	go func() {
		for err := range errs {
			// In a real implementation, you might want to log these errors
			// or provide some way for the caller to access them
			_ = err
		}
	}()

	// Monitor for shutdown
	go func() {
		select {
		case <-p.shutdown:
			// Close the request channel to signal shutdown
			close(p.requestCh)
			// Wait for batch processing to complete
			<-p.batchProcessor.Done()
			// Signal that shutdown is complete
			close(p.shutdownDone)
		case <-p.batchProcessor.Done():
			// If batch processing completes on its own, signal shutdown
			close(p.shutdownDone)
		}
	}()

	return nil
}

// Stop gracefully shuts down the pipeline, waiting for any in-flight
// operations to complete.
func (p *RedisPipeline) Stop() error {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return errors.New("pipeline not started")
	}
	p.mu.Unlock()

	// Signal shutdown
	close(p.shutdown)
	// Wait for shutdown to complete
	<-p.shutdownDone

	p.mu.Lock()
	p.started = false
	p.mu.Unlock()

	return nil
}

// Read implements the batch.Source interface. It reads requests from the
// request channel and returns them as batch items.
func (p *RedisPipeline) Read(ctx context.Context) (<-chan interface{}, <-chan error) {
	outCh := make(chan interface{})
	errCh := make(chan error)

	go func() {
		defer close(outCh)
		defer close(errCh)

		for {
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			case req, ok := <-p.requestCh:
				if !ok {
					// Request channel closed, we're done
					return
				}
				// Send the request to the batch processor
				outCh <- req
			}
		}
	}()

	return outCh, errCh
}

// Process implements the batch.Processor interface. It processes a batch
// of Redis requests in a single operation.
func (p *RedisPipeline) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	if len(items) == 0 {
		return items, nil
	}

	// In a real implementation, you would use the Redis pipeline feature here
	// to execute all commands in a single round-trip.
	//
	// For example with go-redis/redis:
	//
	// pipe := p.redisClient.Pipeline()
	// cmds := make(map[uint64]*redis.Cmd)
	//
	// for _, item := range items {
	//     req := item.Data.(*redisRequest)
	//     switch req.op {
	//     case Get:
	//         cmds[item.ID] = pipe.Get(ctx, req.key)
	//     case Set:
	//         cmds[item.ID] = pipe.Set(ctx, req.key, req.value, 0)
	//     // Handle other operations...
	//     }
	// }
	//
	// _, err := pipe.Exec(ctx)
	// if err != nil {
	//     return items, err
	// }
	//
	// for _, item := range items {
	//     req := item.Data.(*redisRequest)
	//     cmd := cmds[item.ID]
	//
	//     var resp redisResponse
	//     resp.value, resp.err = cmd.Result()
	//
	//     // Send the response back to the caller
	//     req.respCh <- &resp
	// }

	// For this demonstration, we'll simulate Redis operations
	for _, item := range items {
		req, ok := item.Data.(*redisRequest)
		if !ok {
			item.Error = errors.New("invalid request type")
			continue
		}

		// Simulate a Redis operation
		resp := &redisResponse{}
		switch req.op {
		case Get:
			// Simulate a GET operation
			resp.value = "simulated-value-for-" + req.key
		case Set:
			// Simulate a SET operation
			resp.value = "OK"
		case Del:
			// Simulate a DEL operation
			resp.value = int64(1)
		case Exists:
			// Simulate an EXISTS operation
			resp.value = int64(1)
		default:
			resp.err = errors.New("unsupported operation")
		}

		// Send the response back to the caller
		req.respCh <- resp
	}

	return items, nil
}

// Get retrieves the value for a key. It returns the value and any error.
func (p *RedisPipeline) Get(ctx context.Context, key string) (interface{}, error) {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return nil, errors.New("pipeline not started")
	}
	p.mu.Unlock()

	// Create the response channel
	respCh := make(chan *redisResponse, 1)

	// Create the request
	req := &redisRequest{
		op:     Get,
		key:    key,
		respCh: respCh,
	}

	// Submit the request to the pipeline
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case p.requestCh <- req:
		// Request submitted successfully
	}

	// Wait for the response
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-respCh:
		return resp.value, resp.err
	}
}

// Set sets a key-value pair. It returns the result and any error.
func (p *RedisPipeline) Set(ctx context.Context, key string, value interface{}) (interface{}, error) {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return nil, errors.New("pipeline not started")
	}
	p.mu.Unlock()

	// Create the response channel
	respCh := make(chan *redisResponse, 1)

	// Create the request
	req := &redisRequest{
		op:     Set,
		key:    key,
		value:  value,
		respCh: respCh,
	}

	// Submit the request to the pipeline
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case p.requestCh <- req:
		// Request submitted successfully
	}

	// Wait for the response
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-respCh:
		return resp.value, resp.err
	}
}

// Del deletes a key. It returns the number of keys deleted and any error.
func (p *RedisPipeline) Del(ctx context.Context, key string) (int64, error) {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return 0, errors.New("pipeline not started")
	}
	p.mu.Unlock()

	// Create the response channel
	respCh := make(chan *redisResponse, 1)

	// Create the request
	req := &redisRequest{
		op:     Del,
		key:    key,
		respCh: respCh,
	}

	// Submit the request to the pipeline
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case p.requestCh <- req:
		// Request submitted successfully
	}

	// Wait for the response
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case resp := <-respCh:
		if resp.err != nil {
			return 0, resp.err
		}
		if val, ok := resp.value.(int64); ok {
			return val, nil
		}
		return 0, errors.New("invalid response type")
	}
}

// Exists checks if a key exists. It returns a boolean and any error.
func (p *RedisPipeline) Exists(ctx context.Context, key string) (bool, error) {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return false, errors.New("pipeline not started")
	}
	p.mu.Unlock()

	// Create the response channel
	respCh := make(chan *redisResponse, 1)

	// Create the request
	req := &redisRequest{
		op:     Exists,
		key:    key,
		respCh: respCh,
	}

	// Submit the request to the pipeline
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case p.requestCh <- req:
		// Request submitted successfully
	}

	// Wait for the response
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case resp := <-respCh:
		if resp.err != nil {
			return false, resp.err
		}
		if val, ok := resp.value.(int64); ok {
			return val > 0, nil
		}
		return false, errors.New("invalid response type")
	}
}
