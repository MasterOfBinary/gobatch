# ChatGPT Instructions

This file contains instructions and guidelines for ChatGPT when working with this project.

## Project Overview
GoBatch is a Go library for batch data processing. It provides infrastructure for processing batches of data with configurable sources, processors, and pipelines. The library allows for flexible batch processing with configurable timing and item count parameters.

**Requirements:** Go 1.18 or later.

**Note:** GoBatch is a version 0 release and is in an unstable state. Compatibility may be broken at any time on the master branch.

## Important Commands
- Format code: `gofmt -w .` (ensure no unformatted files remain)
- Run tests with race detection and coverage: `go test -race -coverprofile=coverage.txt -covermode=atomic ./...`
- Basic lint: `go vet ./...`
- Full lint: `golangci-lint run --timeout=3m`

## Code Standards
- Follow Go best practices and idiomatic Go patterns
- Ensure all exported functions are properly documented with godoc style comments
- Always include a package-level doc comment that explains the package's purpose
- Document interfaces thoroughly with usage examples
- For methods, include descriptive comments that explain parameters, return values, and behavior
- Write tests for new functionality
- Maintain backward compatibility where possible

## Documentation Style
- Use godoc style comments for all exported types, functions, methods, and constants
- Package documentation should provide an overview of the package purpose and content
- Interface documentation should include usage examples
- Method documentation should explain what the method does, its parameters, return values, and any side effects
- Follow the format seen in existing files (e.g., batch/batch.go)
- Include practical examples in documentation

## File Structure
- `/batch`: Core batch processing functionality, includes the main Batch type, configuration, errors, and helper functions
- `/processor`: Different processors for data manipulation (Transform, Filter, Channel, Error, Nil)
- `/source`: Data sources for batch processing (Channel, Error, Nil)
- `/doc.go`: Package-level documentation
- `/example_test.go`: Top-level usage examples

## Key Concepts
- **Batch**: Main type that orchestrates the batch processing pipeline
- **Source**: Interface for data providers that read from various origins
- **Processor**: Interface for components that process batches of items
- **Item**: Represents a single data item flowing through the pipeline with unique ID and optional error
- **Config**: Interface for controlling batch timing and item count parameters
  - **ConstantConfig**: Static, unchanging configuration
  - **DynamicConfig**: Runtime-adjustable configuration that can be updated while processing
- **BufferConfig**: Controls internal channel buffer sizes for performance tuning

## Built-in Components

### Processors
- **Transform**: Transforms item data with a custom function
- **Filter**: Filters items based on a predicate function
- **Channel**: Writes item data to an output channel
- **Error**: Simulates processor errors (for testing)
- **Nil**: Passes items through unchanged (for benchmarking)

### Sources
- **Channel**: Uses Go channels as data sources
- **Error**: Simulates error-only sources (for testing)
- **Nil**: Emits no data (for timing tests)

## Helper Functions
- `IgnoreErrors`: Drains the error channel in the background
- `CollectErrors`: Collects all errors into a slice after batch processing finishes
- `RunBatchAndWait`: Starts a batch, waits for completion, and returns all collected errors
- `ExecuteBatches`: Runs multiple batches concurrently and collects all errors

## Error Handling
- Errors are wrapped to indicate their origin (SourceError, ProcessorError)
- Processors should respect context cancellation
- Items with errors are tracked individually through the Error field
- Batch processing continues despite individual item errors
- Use `errors.As` to check error types (SourceError, ProcessorError)
