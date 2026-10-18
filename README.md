# GoBatch

[![Go](https://github.com/MasterOfBinary/gobatch/actions/workflows/go.yml/badge.svg)](https://github.com/MasterOfBinary/gobatch/actions/workflows/go.yml)
[![codecov](https://codecov.io/gh/MasterOfBinary/gobatch/branch/master/graph/badge.svg)](https://codecov.io/gh/MasterOfBinary/gobatch)
[![PkgGoDev](https://pkg.go.dev/badge/github.com/MasterOfBinary/gobatch)](https://pkg.go.dev/github.com/MasterOfBinary/gobatch)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

GoBatch is a Go library for processing items in batches.
You provide a source that produces items and processors that handle each batch.
GoBatch groups items into batches by count and time, runs each batch through the
processors, and reports errors on a channel.

**Note:** GoBatch is a version 0 release.
The API is unstable and may change without notice on the master branch.
If you need a stable API, wait for version 1.
See [CHANGELOG.md](./CHANGELOG.md) for release history.

## Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [How It Works](#how-it-works)
- [Configuration](#configuration)
- [Error Handling](#error-handling)
- [Batch Lifecycle](#batch-lifecycle)
- [Built-in Sources and Processors](#built-in-sources-and-processors)
- [Helper Functions](#helper-functions)
- [Documentation](#documentation)
- [Testing](#testing)
- [Contributing](#contributing)
- [License](#license)

## Installation

GoBatch requires Go 1.18 or later.

```bash
go get github.com/MasterOfBinary/gobatch
```

The library has three packages:

- `github.com/MasterOfBinary/gobatch/batch`: the core batching engine.
- `github.com/MasterOfBinary/gobatch/source`: built-in sources.
- `github.com/MasterOfBinary/gobatch/processor`: built-in processors.

## Quick Start

This example reads five integers from a channel, doubles each one, and prints
the results.

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/processor"
	"github.com/MasterOfBinary/gobatch/source"
)

func main() {
	// Collect all five items in a single batch. Batches run in separate
	// goroutines, so using one batch keeps the output order stable.
	b := batch.New[int](batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 5,
		MaxItems: 5,
	}))

	// Create an input channel and wrap it with source.Channel.
	ch := make(chan int)
	src := &source.Channel[int]{Input: ch}

	// First processor: double each number.
	doubleProc := &processor.Transform[int]{
		Func: func(data int) (int, error) {
			return data * 2, nil
		},
	}

	// Second processor: print each number.
	printProc := &processor.Transform[int]{
		Func: func(data int) (int, error) {
			fmt.Println(data)
			return data, nil
		},
	}

	ctx := context.Background()

	// Start processing. The processors run in the order given.
	errs, err := b.Go(ctx, src, doubleProc, printProc)
	if err != nil {
		log.Fatal(err)
	}

	// Drain the error channel. This example discards the errors.
	// See Error Handling for other options.
	batch.IgnoreErrors(errs)

	// Send the items, then close the channel to end the input.
	go func() {
		for i := 1; i <= 5; i++ {
			ch <- i
		}
		close(ch)
	}()

	// Wait for processing to finish.
	<-b.Done()
}
```

Output:

```
2
4
6
8
10
```

The five items form a single batch, so they print in order.
In general, GoBatch processes batches concurrently, one goroutine per batch.
The order in which different batches finish is not guaranteed.

## How It Works

A pipeline is built from these types, all in the `batch` package:

- `Batch[T]` runs the pipeline for items of type `T`.
  Create one with `batch.New[T](config)` and start it with `Go`.
- `Source[T]` produces items.
  Its `Read(ctx)` method returns two channels, `(<-chan T, <-chan error)`.
  A source must return two non-nil channels and must close both of them when
  reading is finished or the context is canceled.
- `Processor[T]` handles a batch of items.
  Its `Process(ctx, items)` method receives `[]*Item[T]`, returns the items for
  the next processor, and should respect context cancellation.
- `Item[T]` is a single item in the pipeline.
  `ID` is a unique identifier set by GoBatch; processors must not change it.
  `Data` is the payload; processors may change it.
  `Error` can be set by a processor to mark that one item as failed.
- `Config` supplies the batch size and timing values described in
  [Configuration](#configuration).

Processing proceeds in four steps:

1. **Read.** `Go` calls `Source.Read`.
   Each value from the data channel becomes an `Item` with a unique `ID`.
   Each value from the error channel is forwarded to the caller, wrapped in a
   `*SourceError`.
2. **Group.** Items are collected until the configuration says a batch is ready.
   Collection of the next batch starts as soon as the previous batch has been
   sent for processing.
   It does not wait for that batch to finish.
3. **Process.** Each batch runs in its own goroutine.
   Within a batch, the processors run in the order passed to `Go`, and each
   processor receives the output of the previous one.
   Different batches run at the same time, so a processor can be called
   concurrently for different batches.
   A processor that keeps state must be safe for concurrent use.
4. **Report.** Processor errors and per-item errors are sent on the error
   channel returned by `Go`; see [Error Handling](#error-handling).
   When the source has closed both of its channels and every batch has
   finished, GoBatch closes the `Done` channel and then the error channel.

## Configuration

The `ConfigValues` struct controls when a batch is sent for processing:

- `MinItems`: the fewest items to collect before a batch can be sent.
- `MaxItems`: the most items in one batch. `0` means no maximum.
- `MinTime`: the shortest time to wait before sending a batch.
- `MaxTime`: the longest time to wait before sending a batch. `0` means no
  maximum.

A batch is sent as soon as one of these happens:

- `MaxItems` items have been collected (when `MaxItems` is nonzero).
- `MaxTime` has elapsed and at least one item has been collected (when
  `MaxTime` is nonzero).
  If `MaxTime` elapses before any item arrives, the timer restarts.
- The input ends.
  Any remaining items are sent, even if there are fewer than `MinItems` or
  `MinTime` has not elapsed.
- At least `MinItems` items have been collected and `MinTime` has elapsed.

In short, the maximum limits and the end of the input override the minimums.
Counts and timers restart for each batch.
They are measured from the time the previous batch was sent for processing.

Before each batch, GoBatch normalizes the values:

- If `MinItems` is `0`, it is treated as `1`.
- If `MaxItems` is nonzero and `MinItems` is greater than `MaxItems`,
  `MinItems` is lowered to `MaxItems`.
- If `MaxTime` is nonzero and `MinTime` is greater than `MaxTime`, `MinTime`
  is lowered to `MaxTime`.

Passing a `nil` config to `New` uses the defaults: `MinItems` of `1` and no
other limits, so each item is processed as soon as it is read.
Creating a batch as `&batch.Batch[T]{}` behaves the same way, but `New` is the
usual way to create one.

### Constant Configuration

Use `ConstantConfig` when the values do not change.

```go
config := batch.NewConstantConfig(&batch.ConfigValues{
	MinItems: 10,
	MaxItems: 100,
	MinTime:  50 * time.Millisecond,
	MaxTime:  500 * time.Millisecond,
})

b := batch.New[int](config)
```

### Dynamic Configuration

Use `DynamicConfig` when the values need to change while processing is
running, for example in response to load.
`DynamicConfig` is safe to update from other goroutines while a batch is
running.

```go
dynConfig := batch.NewDynamicConfig(&batch.ConfigValues{
	MinItems: 10,
	MaxItems: 100,
	MinTime:  50 * time.Millisecond,
	MaxTime:  500 * time.Millisecond,
})

b := batch.New[int](dynConfig)

// ... start processing with b.Go ...

// Later, replace all four values at once:
dynConfig.Update(batch.ConfigValues{
	MinItems: 20,
	MaxItems: 200,
	MinTime:  100 * time.Millisecond,
	MaxTime:  time.Second,
})
```

`Update` replaces all four values under a single lock.
To change just one pair of values, use `UpdateBatchSize(minItems, maxItems)`
or `UpdateTiming(minTime, maxTime)`.
Calling those two methods one after the other takes two separate locks, so a
batch can start between them and see one new pair of values together with one
old pair.
Use `Update` when the size and timing values must change together.

A configuration change takes effect from the next batch.
GoBatch reads the configuration once, when it starts collecting a batch.
The batch that is currently being collected keeps the values it started with.

### Buffer Configuration

`WithBufferConfig` sets the sizes of the internal channels.
Call it before `Go`; calling it after `Go` has started panics.

```go
b := batch.New[int](config).WithBufferConfig(batch.BufferConfig{
	ItemBufferSize:  1000, // Buffer for items waiting to be grouped.
	ErrorBufferSize: 500,  // Buffer for errors waiting to be read.
})
```

`ItemBufferSize` defaults to `batch.DefaultItemBufferSize` (100), and
`ErrorBufferSize` defaults to `batch.DefaultErrorBufferSize` (100).
Leaving a field at `0` uses its default.
A larger error buffer only holds more unread errors.
It does not remove the need to read the error channel; see
[Error Handling](#error-handling).

### Custom Config

You can supply your own configuration by implementing the `Config` interface,
which has a single method, `Get() batch.ConfigValues`.
`Get` can be called while processing is running, so it must be safe for
concurrent use if its values can change.
The returned values are normalized as described above.

## Error Handling

`Go` returns an error channel and a start error: `(<-chan error, error)`.
Check the start error first; see [Batch Lifecycle](#batch-lifecycle).
All errors that occur during processing are sent on the error channel.
They come from three places:

1. **Source errors.** Values sent on the error channel returned by
   `Source.Read`.
   Each one is wrapped in a `*batch.SourceError`.
2. **Processor errors.** The error returned by a `Process` call.
   It is wrapped in a `*batch.ProcessorError`.
   The batch still goes through any remaining processors.
3. **Item errors.** The `Error` field set on individual items by a processor.
   After the last processor runs, each item error is reported, wrapped in a
   `*batch.ProcessorError`.

A source that returns a nil channel is reported as a plain error, without a
wrapper.
An error in one item or one batch does not stop later batches from being
processed.
Use `errors.As` to tell source errors from processor errors, and call
`Unwrap()` on the result to get the underlying error.

**Always drain the error channel.**
Errors are sent with blocking sends.
The channel is buffered (100 errors by default; see
[Buffer Configuration](#buffer-configuration)), but once the buffer fills,
the goroutine sending an error blocks and processing stops making progress.
Canceling the context does not unblock a blocked send.
Read the channel until it closes: range over it, pass it to
`batch.CollectErrors`, or pass it to `batch.IgnoreErrors` if you do not need
the errors.
Never wait on `<-b.Done()` unless something is already reading the error
channel.

Note that the `Done` channel closes slightly before the error channel closes.
If errors are read in a separate goroutine, waiting on `Done` alone does not
guarantee that the goroutine has handled every error.
Ranging over the error channel until it closes, or using `CollectErrors`,
does guarantee that.

The simplest pattern ranges over the channel directly.
The loop blocks until processing is complete:

```go
errs, err := b.Go(ctx, src, proc)
if err != nil {
	log.Fatal(err)
}
for err := range errs {
	var srcErr *batch.SourceError
	var procErr *batch.ProcessorError
	switch {
	case errors.As(err, &srcErr):
		log.Printf("source error: %v", srcErr.Unwrap())
	case errors.As(err, &procErr):
		log.Printf("processor error: %v", procErr.Unwrap())
	default:
		log.Printf("error: %v", err)
	}
}
```

Two helper functions cover common cases without an explicit loop.
Use one or the other.
Each of the next two examples creates its own `Batch`, because a `Batch` can
be started only once.

To collect every error into a slice, start the batch with `Go` and then call
`CollectErrors`.
`CollectErrors` blocks until the error channel closes, so there is no need
to wait on `Done` afterwards:

```go
b := batch.New[int](config)
pipeErrs, err := b.Go(ctx, src, proc)
if err != nil {
	log.Fatal(err)
}
errs := batch.CollectErrors(pipeErrs)
for _, e := range errs {
	log.Printf("error: %v", e)
}
```

Or let `RunBatchAndWait` do all of it.
It starts the batch, waits for it to finish, and returns every error.
If the batch cannot start, the start error is returned as the single element
of the slice:

```go
b := batch.New[int](config)
errs := batch.RunBatchAndWait(ctx, b, src, proc)
for _, e := range errs {
	log.Printf("error: %v", e)
}
```

If you do not need the errors at all, drain the channel with
`batch.IgnoreErrors(errs)` and then wait on `<-b.Done()`, as in
[Quick Start](#quick-start).

## Batch Lifecycle

A `Batch` is single-use.
Call `Go` exactly once per `Batch`.
Calling `Go` again does not start a second run.
Instead, it returns `batch.ErrBatchUsed` as the start error.
To run again, create a new `Batch` with `batch.New`.

`Go` also returns `batch.ErrNilSource` when the source is `nil`.
Both start errors can be checked with `errors.Is`.
When `Go` returns a start error, the error channel it returns is non-nil and
already closed, so ranging over it is safe even if the error is not checked.
`RunBatchAndWait` and `ExecuteBatches` report a start error in the slice they
return instead; see [Helper Functions](#helper-functions).

Other rules for a `Batch`:

- Call `WithBufferConfig` before `Go`.
  Calling it after `Go` has started panics.
- Nil processors passed to `Go` are ignored.
- `Done` returns a channel that is closed when processing finishes.
  Before `Go` is called, `Done` returns an already-closed channel, so only
  wait on `Done` after a successful `Go`, and only while the error channel
  is being drained.
  See [Error Handling](#error-handling).

Context cancellation is cooperative.
GoBatch does not discard items it has already read from the source; those
items are still processed.
Sources are expected to close their channels when the context is canceled,
and processors should check the context and return early.

## Built-in Sources and Processors

The `source` and `processor` packages provide ready-made implementations of
`batch.Source[T]` and `batch.Processor[T]`.
You can also write your own by implementing those interfaces.

### Sources

- `source.Channel[T]` forwards items from its `Input` channel until the
  channel is closed or the context is canceled.
  Close `Input` to end the stream; the source does not close it.
  A nil `Input` produces no items.
  The `BufferSize` field sets the buffer of the internal output channel and
  defaults to 100.
- `source.Error[T]` emits no items.
  It forwards the non-nil errors from its `Errs` channel until the channel is
  closed or the context is canceled.
  The source does not close `Errs`.
  The `BufferSize` field sets the buffer of the internal error channel and
  defaults to 10.
  It is mainly useful for testing error handling.
- `source.Nil[T]` emits nothing.
  It waits for its `Duration`, or until the context is canceled, and then
  closes its channels.
  A `Duration` of `0` closes them at once.
  It is mainly useful for timing tests.

### Processors

- `processor.Transform[T]` applies its `Func` to the `Data` field of each
  item that does not already have an error.
  A nil `Func` leaves items unchanged.
  If `Func` returns an error, the item's `Error` field is set.
  When `StopOnError` is true, the first such error is also returned at once
  and the remaining items in the batch are not transformed.
- `processor.Filter[T]` keeps the items for which its `Predicate` returns
  true, and drops the rest.
  When `InvertMatch` is true, it keeps the items for which `Predicate`
  returns false instead.
  A nil `Predicate` keeps every item.
  Items that already have an error are always kept, so their error is not
  lost.
- `processor.Channel[T]` sends the `Data` field of each item that does not
  have an error to its `Output` channel.
  If the context is canceled while sending, it stops and returns the
  context error.
  It does not close `Output`.
  A nil `Output` does nothing.
- `processor.Error[T]` simulates errors on items.
  It is mainly useful for testing.
- `processor.Nil[T]` waits for its `Duration` and returns the items
  unchanged.
  A `Duration` of `0` returns at once.
  If the context is canceled during the wait and `MarkCancelled` is true,
  each item is marked with the context error.
  It is mainly useful for timing tests and benchmarks.

## Helper Functions

The `batch` package provides four helpers for common patterns:

- `batch.IgnoreErrors(errs)` starts a goroutine that reads the error channel
  and discards every error.
  Use it after `Go` when you do not need the errors but still need the
  channel drained, for example so you can wait on `Done`.
- `batch.CollectErrors(errs)` reads the error channel until it closes and
  returns the errors as a slice.
  It blocks until processing is complete, so there is no need to wait on
  `Done` afterwards.
- `batch.RunBatchAndWait(ctx, b, src, procs...)` calls `Go` on the given
  batch, waits for it to finish, and returns every error as a slice.
  If the batch cannot start, the start error is returned as the single
  element of the slice.
- `batch.ExecuteBatches(ctx, configs...)` runs several batches at the same
  time and returns the errors from all of them as one slice.
  Each config is a `*batch.BatchConfig[T]`, with fields `B` (the `Batch`),
  `S` (the source), and `P` (the processors).
  All configs in one call must use the same item type `T`.
  A nil config, or a config with a nil `B`, is skipped.
  A start error, such as a nil source, is included in the returned slice.
  Errors from different batches can appear in any order.

```go
errs := batch.ExecuteBatches(ctx,
	&batch.BatchConfig[int]{B: b1, S: src1, P: []batch.Processor[int]{proc1}},
	&batch.BatchConfig[int]{B: b2, S: src2, P: []batch.Processor[int]{proc2}},
)
for _, e := range errs {
	log.Printf("error: %v", e)
}
```

## Documentation

The full API reference, including runnable examples for the `batch`,
`source`, and `processor` packages, is on pkg.go.dev:
[github.com/MasterOfBinary/gobatch](https://pkg.go.dev/github.com/MasterOfBinary/gobatch).

## Testing

Run the tests with the race detector:

```bash
go test -race ./...
```

Formatting and vet should also be clean.
`gofmt -l .` prints nothing when every file is formatted:

```bash
gofmt -l .
go vet ./...
```

Continuous integration runs the same tests with coverage and also runs
`golangci-lint`.

## Contributing

Contributions are welcome.
Open an issue or submit a pull request.
Run the checks in [Testing](#testing) before submitting, and include tests
and godoc comments for any new exported code.

## License

GoBatch is licensed under the MIT License.
See [LICENSE](./LICENSE) for details.
