# GoBatch

[![Build Status](https://travis-ci.org/MasterOfBinary/gobatch.svg?branch=master)](https://travis-ci.org/MasterOfBinary/gobatch)
[![Coverage Status](https://coveralls.io/repos/github/MasterOfBinary/gobatch/badge.svg?branch=master)](https://coveralls.io/github/MasterOfBinary/gobatch?branch=master)
[![PkgGoDev](https://pkg.go.dev/badge/github.com/MasterOfBinary/gobatch)](https://pkg.go.dev/github.com/MasterOfBinary/gobatch)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## How GoBatch Works

GoBatch is a flexible and efficient batch processing library for Go, designed to streamline the processing of large
volumes of data. It provides a framework for batch processing while allowing users to define their own data sources
and processing logic.

**NOTE:** GoBatch is considered a version 0 release and is in an unstable state. Compatibility may be broken at any time on
the master branch. If you need a stable release, wait for version 1.

### Core Components

1. `Source`: An interface implemented by the user to define where data comes from (e.g. a channel, database, API, or file system).
2. `Processor`: An interface implemented by the user to define how batches of data should be processed.
3. `Batch`: The central structure provided by GoBatch that manages the batch processing pipeline.

### The Batch Processing Pipeline

1. **Data Reading**:
    - The `Source` implementation reads data from its origin.
    - Data items are sent to the `Batch` structure via channels in a `PipelineStage`.

2. **Batching**:
    - The `Batch` structure queues incoming items.
    - It determines when to form a batch based on configured criteria (time elapsed, number of items, etc.).

3. **Processing**:
    - When a batch is ready, `Batch` sends it to the `Processor` implementation.
    - The `Processor` performs the user-defined operations on the batch.

4. **Result Handling**:
    - Processed results and any errors are managed by the `Batch` structure.

### Typical Use Cases

GoBatch be applied to a lot of scenarios where processing items in batches is beneficial. Some potential use-cases
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

- Go 1.7 or later

## Key Components

- `Batch`: The main struct that manages the batch processing.
- `Source`: An interface for providing data to be processed.
- `Processor`: An interface for processing batches of data.
- `Config`: An interface for providing configuration values.
- `Item`: A struct representing a single item in the processing pipeline. Each `Item` has a unique ID for traceability.
- `PipelineStage`: A struct containing input and output channels for a single stage of the batch pipeline.

## Basic Usage

Here's a simple example of how to use GoBatch:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/source"
)

// MyProcessor is a Processor that prints items in batches.
type MyProcessor struct{}

// Process prints a batch of items.
func (p *MyProcessor) Process(_ context.Context, ps *batch.PipelineStage) {
	defer ps.Close()

	for range ps.Input {
		// Process the data here...
	}
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

	processor := &MyProcessor{}

	ctx := context.Background()
	errs := batchProcessor.Go(ctx, s, processor)

	// Handle errors
	go func() {
		for err := range errs {
			if srcErr, ok := err.(*batch.SourceError); ok {
				log.Printf("Source error: %v", srcErr.Original())
			} else if procErr, ok := err.(*batch.ProcessorError); ok {
				log.Printf("Processor error: %v", procErr.Original())
			} else {
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

## Error Handling

Errors from both the `Source` and `Processor` are returned on the error channel provided by the `Go` method. These
errors are wrapped in `SourceError` and `ProcessorError` types respectively. You can use type assertions to determine
the origin of the error.

## Utility Functions

- `NextItem`: Helper function for implementing `Source.Read`.
- `IgnoreErrors`: Utility for discarding errors if they're not needed.

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
