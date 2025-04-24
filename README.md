# GoBatch

[![Go](https://github.com/MasterOfBinary/gobatch/actions/workflows/go.yml/badge.svg)](https://github.com/MasterOfBinary/gobatch/actions/workflows/go.yml)
[![Coverage Status](https://coveralls.io/repos/github/MasterOfBinary/gobatch/badge.svg?branch=master)](https://coveralls.io/github/MasterOfBinary/gobatch?branch=master)
[![PkgGoDev](https://pkg.go.dev/badge/github.com/MasterOfBinary/gobatch)](https://pkg.go.dev/github.com/MasterOfBinary/gobatch)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## How GoBatch Works

GoBatch is a flexible and efficient batch processing library for Go, designed to streamline the processing of large
volumes of data. It provides a framework for batch processing while allowing users to define their own data sources
and processing logic.

**NOTE:** GoBatch is considered a version 0 release and is in an unstable state. Compatibility may be broken at any time on
the master branch. If you need a stable release, wait for version 1.

### Latest Release - v0.2.1

Version 0.2.1 fixes several important bugs and improves usability:

- Fixed critical bug where items less than MinItems would not be processed when the source was exhausted
- Added new helper functions for common batch processing operations
- Improved documentation throughout the codebase following Go standards
- Enhanced error handling and reporting

See the [CHANGELOG.md](./CHANGELOG.md) for complete details.

### Core Components

1. `Source`: An interface implemented by the user to define where data comes from (e.g. a channel, database, API, or file system).
2. `Processor`: An interface implemented by the user to define how batches of data should be processed. Multiple processors can be chained together to create a processing pipeline.
3. `Batch`: The central structure provided by GoBatch that manages the batch processing pipeline.

### The Batch Processing Pipeline

1. **Data Reading**:
    - The `Source` implementation reads data from its origin and returns two channels: data and errors.
    - Data items are sent to the `Batch` structure via these channels.

2. **Batching**:
    - The `Batch` structure queues incoming items.
    - It determines when to form a batch based on configured criteria (time elapsed, number of items, etc.).

3. **Processing**:
    - When a batch is ready, `Batch` sends it to the `Processor` implementation(s).
    - Each processor in the chain performs operations on the batch and passes the results to the next processor.
    - The `Processor` performs user-defined operations on the batch and returns processed items.
    - Individual item errors are tracked within the `Item` struct.

4. **Result Handling**:
    - Processed results and any errors are managed by the `Batch` structure.
    - Errors can come from the Source, Processor, or individual items.

### Typical Use Cases

GoBatch can be applied to a lot of scenarios where processing items in batches is beneficial. Some potential use-cases
include:

- Database Operations: Optimize database inserts, updates, or reads by batching operations.
- Log Processing: Efficiently process log entries in batches for analysis or storage.
- File Processing: Process large files in manageable chunks.
- Cache Updates: Reduce network overhead by batching cache update operations.
- Message Queue Consumption: Efficiently process messages from queues in batches.
- Bulk Data Validation: Validate large datasets in parallel batches.

By batching operations, you can reduce network overhead, optimize resource utilization, and improve overall system
performance.

## Installation

To download, run

    go get github.com/MasterOfBinary/gobatch

## Requirements

- Go 1.18 or later

## Key Components

- `Batch`: The main struct that manages the batch processing.
- `Source`: An interface for providing data to be processed by implementing `Read(ctx) (<-chan interface{}, <-chan error)`.
- `Processor`: An interface for processing batches of data by implementing `Process(ctx, []*Item) ([]*Item, error)`.
- `Config`: An interface for providing configuration values.
- `Item`: A struct representing a single item in the processing pipeline. Each `Item` has a unique ID for traceability and an `Error` field for tracking item-specific errors.

### Built-in Processors

GoBatch includes several built-in processors for common tasks:

1. **Filter**: Filters items based on a predicate function.
   - Configurable with a custom `Predicate` function.
   - Supports `InvertMatch` option to remove matching items instead of keeping them.

2. **Transform**: Transforms item data using a custom function.
   - Applies a transformation function to each item's `Data` field.
   - Provides `ContinueOnError` option to control behavior when transformations fail.
   - Skips items that already have errors set.

3. **Error**: Simulates processor errors with configurable failure rates.
   - Useful for testing error handling in your processing pipeline.
   - Can be configured to fail at specific rates or patterns.

4. **Nil**: For testing timing behavior without modifying items.
   - Passes items through without changes.
   - Useful for benchmarking and timing tests.

### Built-in Sources

GoBatch includes several built-in source implementations:

1. **Channel**: Uses existing Go channels as batch sources.
   - Supports `BufferSize` configuration for controlling buffering.
   - Allows easy integration with existing channel-based code.

2. **Error**: Simulates error-only sources without producing data.
   - Useful for testing error handling in your processing pipeline.
   - Supports `BufferSize` configuration and filters out nil errors.

3. **Nil**: For testing timing behavior without emitting any data.
   - Properly handles zero/negative durations.
   - Uses timers correctly for precise timing tests.

### Helper Functions

GoBatch provides several helper functions for common operations:

1. **IgnoreErrors**: Safely drains the error channel without needing to process errors.

2. **CollectErrors**: Collects all errors from the error channel into a slice for later processing.

3. **RunBatchAndWait**: Runs a batch and waits for completion, collecting all errors in one step.

4. **ExecuteBatches**: Runs multiple batches concurrently and collects all errors.

```go
// Example using RunBatchAndWait
errors := batch.RunBatchAndWait(ctx, batchProcessor, source, processor1, processor2)
if len(errors) > 0 {
    // Handle errors
}

// Example using ExecuteBatches
errors := batch.ExecuteBatches(ctx,
    &batch.BatchConfig{B: batch1, S: source1, P: []batch.Processor{proc1}},
    &batch.BatchConfig{B: batch2, S: source2, P: []batch.Processor{proc2}},
)
```

## Basic Usage

Here's a simple example of how to use GoBatch:

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/source"
)

// MyProcessor is a Processor that prints items in batches.
type MyProcessor struct{}

// Process prints a batch of items and returns them.
func (p *MyProcessor) Process(_ context.Context, items []*batch.Item) ([]*batch.Item, error) {
	for _, item := range items {
		fmt.Printf("Processing item %d: %v\n", item.ID, item.Data)
		
		// Optionally set an error on the item
		// item.Error = fmt.Errorf("processing error")
	}
	return items, nil
}

// AnotherProcessor demonstrates chaining processors together.
type AnotherProcessor struct{}

func (p *AnotherProcessor) Process(_ context.Context, items []*batch.Item) ([]*batch.Item, error) {
	for _, item := range items {
		if val, ok := item.Data.(int); ok {
			// Modify the data
			item.Data = val * 2
		}
	}
	return items, nil
}

func main() {
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 50,
		MaxItems: 200,
		MinTime:  100 * time.Millisecond,
		MaxTime:  500 * time.Millisecond,
	})

	batchProcessor := batch.New(config)

	// Use the provided Channel source
	ch := make(chan interface{})
	s := &source.Channel{Input: ch}

	// Create multiple processors to chain together
	processor1 := &MyProcessor{}
	processor2 := &AnotherProcessor{}

	ctx := context.Background()
	// Pass multiple processors to create a processing pipeline
	errs := batchProcessor.Go(ctx, s, processor1, processor2)

	// Handle errors
	go func() {
		for err := range errs {
			var srcErr *batch.SourceError
			var procErr *batch.ProcessorError
			
			switch {
			case errors.As(err, &srcErr):
				log.Printf("Source error: %v", srcErr.Err)
			case errors.As(err, &procErr):
				log.Printf("Processor error: %v", procErr.Err)
			default:
				log.Printf("Error: %v", err)
			}
		}
	}()

	// Simulate data input
	go func() {
		for i := 0; i < 1000; i++ {
			ch <- i
		}
		close(ch)
	}()

	// Wait for completion
	<-batchProcessor.Done()
	fmt.Println("Batch processing completed")
}

```

## Configuration

The `Config` interface allows for flexible configuration of the batch processing behavior. You can use the provided
`ConstantConfig` for static configuration, or implement your own `Config` for dynamic behaviour.

Configuration options include:

- `MinItems`: Minimum number of items to process in a batch.
- `MaxItems`: Maximum number of items to process in a batch.
- `MinTime`: Minimum time to wait before processing a batch.
- `MaxTime`: Maximum time to wait before processing a batch.

The configuration is automatically adjusted to keep it consistent:

- If `MinItems` is greater than `MaxItems`, `MaxItems` will be set to `MinItems`.
- If `MinTime` is greater than `MaxTime`, `MaxTime` will be set to `MinTime`.

```go
config := batch.NewConstantConfig(&batch.ConfigValues{
    MinItems:    10,
    MaxItems:    100,
    MinTime:     50 * time.Millisecond,
    MaxTime:     500 * time.Millisecond,
})

batchProcessor := batch.New(config)
```

**Important Note (v0.2.1+):** When a Source is exhausted, all remaining items will be processed even if there are fewer than MinItems. This ensures no data is lost when the input stream ends.

## Error Handling

Errors can come from three sources:

1. **Source errors**: Returned on the error channel from `Source.Read()`.
2. **Processor errors**: Returned from `Processor.Process()`.
3. **Item-specific errors**: Set on individual items via the `Item.Error` field.

All errors are reported through the error channel returned by the `Go` method. These errors are wrapped in `SourceError` and `ProcessorError` types respectively.

Since GoBatch now requires Go 1.18+, it's recommended to use `errors.As` for error type checking:

```go
// Don't forget to import the "errors" package
import (
	"errors"
	"github.com/MasterOfBinary/gobatch/batch"
)

// Handle errors
go func() {
	for err := range errs {
		var srcErr *batch.SourceError
		var procErr *batch.ProcessorError
		
		switch {
		case errors.As(err, &srcErr):
			log.Printf("Source error: %v", srcErr.Err)
		case errors.As(err, &procErr):
			log.Printf("Processor error: %v", procErr.Err)
		default:
			log.Printf("Error: %v", err)
		}
	}
}()
```

Here's a simplified error handling approach using the built-in helper functions (v0.2.1+):

```go
// Collect all errors
errors := batch.CollectErrors(batchProcessor.Go(ctx, source, processor))
<-batchProcessor.Done()

// Or use the RunBatchAndWait helper function
errors := batch.RunBatchAndWait(ctx, batchProcessor, source, processor)

// Process errors after completion
for _, err := range errors {
    // Handle error
}
```

## Documentation

See the [pkg.go.dev docs](https://pkg.go.dev/github.com/MasterOfBinary/gobatch) for documentation
and an [example](https://pkg.go.dev/github.com/MasterOfBinary/gobatch/batch#example-package).

## Testing

The package includes a comprehensive test suite. Run the tests with:

    go test github.com/MasterOfBinary/gobatch/...

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the LICENSE file for details.
