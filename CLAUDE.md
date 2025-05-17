# Claude Instructions

This file contains instructions and guidelines for Claude when working with this project.

## Project Overview
GoBatch is a Go library for batch data processing. It provides infrastructure for processing batches of data with configurable sources, processors, and pipelines. The library allows for flexible batch processing with configurable timing and item count parameters.

## Important Commands
- Format code with `gofmt` and ensure no unformatted files remain.
- Run tests with race detection and coverage: `go test -race -coverprofile=coverage.txt -covermode=atomic ./...`
- Lint code with `go vet ./...`
- Run `golangci-lint run --timeout=3m` for additional lint checks.

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
- `/batch`: Core batch processing functionality, includes the main Batch type and configuration
- `/processor`: Different processors for data manipulation (Transform, Filter, Error, Nil)
- `/source`: Data sources for batch processing (Channel, Error, Nil)

## Key Concepts
- **Batch**: Main type that orchestrates the batch processing pipeline
- **Source**: Interface for data providers that read from various origins
- **Processor**: Interface for components that process batches of items
- **Item**: Represents a single data item flowing through the pipeline
- **Config**: Interface for controlling batch timing and item count parameters

## Error Handling
- Errors are wrapped to indicate their origin (SourceError, ProcessorError)
- Processors should respect context cancellation
- Items with errors are tracked individually through the Error field
- Batch processing continues despite individual item errors