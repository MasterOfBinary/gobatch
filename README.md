# GoBatch

[![Go](https://github.com/MasterOfBinary/gobatch/actions/workflows/go.yml/badge.svg)](https://github.com/MasterOfBinary/gobatch/actions/workflows/go.yml)
[![codecov](https://codecov.io/gh/MasterOfBinary/gobatch/branch/master/graph/badge.svg)](https://codecov.io/gh/MasterOfBinary/gobatch)
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

- Fixed a critical bug where items less than MinItems would not be processed when the source was exhausted.
- Added new helper functions for common batch processing operations.
- Improved documentation throughout the codebase following Go standards.
- Enhanced error handling and reporting for better diagnostics.

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
    - Individual item errors are tracked within the `Item` struct.

4. **Result Handling**:
    - Processed results and any errors are managed by the `Batch` structure.
    - Errors can come from the Source, Processor, or individual items.

### Typical Use Cases

GoBatch can be applied to a lot of scenarios where processing items in batches is beneficial. Some potential use-cases
include:

- Database Operations: Optimize inserts, updates, or reads by batching operations.
- Log Processing: Efficiently process log entries in batches for analysis or storage.
- File Processing: Process large files in manageable chunks for better performance.
- Cache Updates: Reduce network overhead by batching cache updates.
- Message Queue Consumption: Process messages from queues in batches.
- Bulk Data Validation: Validate large datasets in parallel batches for faster results.

By batching operations, you can reduce network overhead, optimize resource utilization, and improve overall system
performance.

## Installation

To download, run:

```bash
go get github.com/MasterOfBinary/gobatch
```

## Requirements

- Go 1.18 or later is required.

## Key Components

- `Batch`: The main struct that manages batch processing.
- `Source`: Provides data by implementing `Read(ctx) (<-chan interface{}, <-chan error)`.
- `Processor`: Processes batches by implementing `Process(ctx, []*Item) ([]*Item, error)`.
- `Config`: Provides dynamic configuration values.
- `Item`: Represents a single data item with a unique ID and an optional error.

### Built-in Processors

- **Filter**: Filters items based on a predicate function.
- **Transform**: Transforms item data with a custom function.
- **Error**: Simulates processor errors for testing.
- **Nil**: Passes items through unchanged for benchmarking.

### Built-in Sources

- **Channel**: Uses Go channels as sources.
- **Error**: Simulates error-only sources for testing.
- **Nil**: Emits no data for timing tests.

### Helper Functions

- `IgnoreErrors`: Drains the error channel in the background, allowing you to call `Done()` without handling errors immediately.
- `CollectErrors`: Collects all errors into a slice after batch processing finishes.
- `RunBatchAndWait`: Starts a batch, waits for completion, and returns all collected errors.
- `ExecuteBatches`: Runs multiple batches concurrently and collects all errors into a single slice.

## Basic Usage

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/processor"
	"github.com/MasterOfBinary/gobatch/source"
)

func main() {
	// Create a batch processor with simple config
	b := batch.New(batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 2,
		MaxItems: 5,
		MinTime:  10 * time.Millisecond,
		MaxTime:  100 * time.Millisecond,
	}))

	// Create an input channel
	ch := make(chan interface{})

	// Wrap it with a source.Channel
	src := &source.Channel{Input: ch}

	// First processor: double each number
	doubleProc := &processor.Transform{
		Func: func(data interface{}) (interface{}, error) {
			if v, ok := data.(int); ok {
				return v * 2, nil
			}
			return data, nil
		},
	}

	// Second processor: print each processed number
	printProc := &processor.Transform{
		Func: func(data interface{}) (interface{}, error) {
			fmt.Println(data)
			return data, nil
		},
	}

	ctx := context.Background()

	// Start batch processing with processors chained
	errs := b.Go(ctx, src, doubleProc, printProc)

	// Ignore errors for this simple example
	batch.IgnoreErrors(errs)

	// Send some items to the input channel
	go func() {
		for i := 1; i <= 5; i++ {
			ch <- i
		}
		close(ch)
	}()

	// Wait for processing to complete
	<-b.Done()
}
```

**Expected output:**

```
2
4
6
8
10
```

## Examples

GoBatch ships with several runnable examples located in the `batch` directory.
Each example can be executed with `go test ./batch -run Example`.

- `example_test.go` – basic usage with two processors.
- `example_simple_processor_test.go` – implementing a custom processor.
- `example_processor_chain_test.go` – chaining multiple processors together.
- `example_error_handling_test.go` – capturing and reporting errors.
- `example_custom_config_test.go` – custom dynamic configuration logic.
- `example_dynamic_config_test.go` – adjusting configuration at runtime.
- `example_collect_results_test.go` – using `processor.Collect` to gather results.

## Configuration

GoBatch supports flexible configuration through the `Config` interface, which defines how batches are formed based on size and timing rules.

You can choose between:
- **`ConstantConfig`** for static, unchanging settings.
- **`DynamicConfig`** for runtime-adjustable settings that can be updated while processing.

Configuration options include:

- `MinItems`: Minimum number of items to process in a batch.
- `MaxItems`: Maximum number of items to process in a batch.
- `MinTime`: Minimum time to wait before processing a batch.
- `MaxTime`: Maximum time to wait before processing a batch.

The configuration is automatically adjusted to keep it consistent:

- If `MinItems` > `MaxItems`, `MaxItems` will be set to `MinItems`.
- If `MinTime` > `MaxTime`, `MaxTime` will be set to `MinTime`.
### Example: Constant Configuration

```go
config := batch.NewConstantConfig(&batch.ConfigValues{
    MinItems: 10,
    MaxItems: 100,
    MinTime:  50 * time.Millisecond,
    MaxTime:  500 * time.Millisecond,
})

batchProcessor := batch.New(config)
```

## Error Handling

Errors can come from three sources:

1. **Source errors**: Errors returned from `Source.Read()`.
2. **Processor errors**: Errors returned from `Processor.Process()`.
3. **Item-specific errors**: Errors set on individual `Item.Error` fields.

All errors are reported through the error channel returned by the `Go` method.

Example error handling:

```go
import (
	"errors"
	"github.com/MasterOfBinary/gobatch/batch"
)

go func() {
	for err := range errs {
		var srcErr *batch.SourceError
		var procErr *batch.ProcessorError
		switch {
		case errors.As(err, &srcErr):
			log.Printf("Source error: %v", srcErr.Unwrap())
		case errors.As(err, &procErr):
			log.Printf("Processor error: %v", procErr.Unwrap())
		default:
			log.Printf("Error: %v", err)
		}
	}
}()
```

Or using helper functions:

```go
// Collect all errors
errs := batch.CollectErrors(batchProcessor.Go(ctx, source, processor))
<-batchProcessor.Done()

// Or use the RunBatchAndWait helper
errs := batch.RunBatchAndWait(ctx, batchProcessor, source, processor)

for _, err := range errs {
    // Handle error
}
```

## Documentation

See the [pkg.go.dev docs](https://pkg.go.dev/github.com/MasterOfBinary/gobatch) for documentation
and examples.

## Testing

Run tests with:

```bash
go test github.com/MasterOfBinary/gobatch/...
```

## Contributing

Contributions are welcome! Feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the LICENSE file for details.
