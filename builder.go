package gobatch

import "time"

// BatchBuilder is a struct that can create a Batch implementation.
type BatchBuilder struct {
	minTime         time.Duration
	minItems        uint64
	maxTime         time.Duration
	maxItems        uint64
	readConcurrency uint64
}

// NewBuilder returns a default BatchBuilder, which creates a Batch
// implementation based on the variables specified by the With methods.
// The With methods do not modify the BatchBuilder they operate on, and
// instead return a new BatchBuilder based on the original.
//
// The default BatchBuilder creates a Batch implementation where items
// are processed as soon as they are retrieved from the source. Reading
// is done by a single, looping goroutine, and processing is done in the
// background using as many goroutines as necessary with no limit.
//
// Essentially, it runs a couple simple loops that look like this (although
// in reality it's a little more complicated):
//
//    ch := make(chan interface)
//    go func() {
//      for {
//        if err := source.Read(ctx, ch); err != nil {
//          errs <- err
//        }
//      }
//    }
//    for {
//      if item, ok := <-ch; ok {
//        go func() {
//          err := processor.Process(ctx, []interface{}{item})
//          if err != nil {
//            errs <- err
//          }
//        }
//      }
//    }
func NewBuilder() *BatchBuilder {
	return &BatchBuilder{
		readConcurrency: 1,
	}
}

// WithMinItems returns a BatchBuilder that creates a Batch implementation
// with specified minimum number of items. Items will not be processed until
// the minimum number of items has been read. The only exception is if
// a max time has been specified and that time is reached before the minimum
// number of items has been read.
func (b *BatchBuilder) WithMinItems(minItems uint64) *BatchBuilder {
	newBuilder := *b
	newBuilder.minItems = minItems
	return &newBuilder
}

// WithMinTime returns a BatchBuilder that creates a Batch implementation
// with specified minimum amount of time. Items will not be processed until
// the minimum time has passed. The only exception to this is if a max number
// of items has been specified; they will be processed as soon as that max
// is reached.
func (b *BatchBuilder) WithMinTime(minTime time.Duration) *BatchBuilder {
	newBuilder := *b
	newBuilder.minTime = minTime
	return &newBuilder
}

// WithMaxItems returns a BatchBuilder that creates a Batch implementation
// with specified maximum number of items. Once that number of items is
// available for processing, they will be processed whether or not any
// specified min time has been reached.
func (b *BatchBuilder) WithMaxItems(maxItems uint64) *BatchBuilder {
	newBuilder := *b
	newBuilder.maxItems = maxItems
	return &newBuilder
}

// WithMaxTime returns a BatchBuilder that creates a Batch implementation
// with specified maximum amount of time. Once that time has been reached,
// items will be processed whether or not the minimum number of items
// is available.
func (b *BatchBuilder) WithMaxTime(maxTime time.Duration) *BatchBuilder {
	newBuilder := *b
	newBuilder.maxTime = maxTime
	return &newBuilder
}

// WithReadConcurrency returns a BatchBuilder that creates a Batch
// implementation with a specified number of goroutines for reading from
// the source. Each goroutine continuously calls Read to get the latest
// items for processing.
func (b *BatchBuilder) WithReadConcurrency(concurrency uint64) *BatchBuilder {
	newBuilder := *b
	newBuilder.readConcurrency = concurrency
	return &newBuilder
}

func (b *BatchBuilder) Batch() Batch {
	return &batchImpl{
		minItems:        b.minItems,
		minTime:         b.minTime,
		maxItems:        b.maxItems,
		maxTime:         b.maxTime,
		readConcurrency: b.readConcurrency,
	}
}
