# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

Note: This project is in early development. The API may change without warning in any 0.x version.

## [Unreleased]

## [0.4.0] - 2025-11-21

This release addresses critical bugs affecting stability and timing, aligns configuration defaults with documentation, and introduces configurable buffer sizes for performance tuning.

### Added

- **Performance:** Added `WithBufferConfig` option and `BufferConfig` struct to allow customizing internal channel buffer sizes (Items, IDs, Errors). This allows for fine-tuning performance based on specific workload requirements.

### Fixed

- **Critical:** Fixed a busy loop in `doReader` that consumed 100% CPU when one channel closed before the other.
- **Critical:** Fixed `MaxTime` timer logic in `waitForItems` to properly restart the timer if it expired on an empty batch, ensuring latency guarantees are respected after idle periods.

### Changed

- **BREAKING:** Renamed `ContinueOnError` to `StopOnError` in `Transform` processor. The default zero-value (`false`) now correctly implies "Continue Processing", matching the intended behavior.

## [0.3.0] - 2025-05-17

This release introduces new processors, dynamic configuration support, and significant documentation improvements. It also standardizes code formatting and enhances error handling.

### Added

- New `Channel` processor for forwarding item data to output channels.
- `DynamicConfig`, a thread-safe, runtime-adjustable configuration for batch processing.
- Full test coverage for `ConstantConfig` and `DynamicConfig`.
- Comprehensive package-level documentation in all packages.
- Detailed usage examples for core components.
- Pre-closed channel for `Done()` method to prevent blocking on nil channels.
- CI enforcement of standard code formatting with gofmt.

### Fixed

- Fixed data race conditions in `example_dynamic_config_test.go` with proper mutex protection.
- Fixed `Done()` method to always return a non-nil channel.
- Fixed various linter warnings throughout the codebase.
- Ensured proper error handling in example code.

### Improved

- Standardized code formatting with gofmt across the entire codebase.
- Enhanced inline documentation for all packages and key types.
- Modernized CI pipeline with improved testing.
- Better test coverage for edge cases and error scenarios.
- Renamed gobatch.go to doc.go to follow Go conventions.


## [0.2.1] - 2025-04-25

This release focuses on robustness, developer experience, and error handling. It introduces new helper functions, improves error handling throughout the codebase, and simplifies the API by moving some functionality to helper functions.

### Added

- New helper functions for common batch processing tasks:
  - `CollectErrors` for collecting errors from an error channel into a slice.
  - `RunBatchAndWait` for running a batch and waiting for completion in one step.
  - `ExecuteBatches` for running multiple batches concurrently and collecting all errors.
- Comprehensive handling of edge cases:
  - Proper handling of nil processors, which are now filtered out automatically.
  - Proper error handling for nil sources and sources returning nil channels.
  - Detection and handling of very small time values.
  - Support for empty item slices and zero configuration values.
- Extensive test coverage for all edge cases and error scenarios.
- Better documentation and examples for all public APIs.
- Improved documentation comments throughout the codebase following Go standards:
  - Complete sentences with proper punctuation.
  - Comments begin with the entity name being documented.
  - Consistent formatting for code blocks and examples.
  - Detailed documentation for struct fields, methods, and interfaces.

### Changed

- Simplified API by removing the `Batch.Wait()` method in favor of the `RunBatchAndWait` helper function.
- Improved error reporting with more specific error messages.
- Enhanced error handling throughout the codebase for better diagnostics.
- Better context cancellation support and testing.
- Code structure reorganized to be more maintainable and testable.

### Fixed

- Fixed critical bug where items remaining in the pipeline would not be processed if fewer than MinItems when the source was exhausted.
- Fixed potential issues with nil sources and nil processors.
- Fixed handling of timing-dependent tests to make them more reliable.
- Fixed error handling to properly identify and wrap errors from different sources.
- Improved synchronization in concurrent processing scenarios.

## [0.2.0] - 2025-04-24

This release brings major improvements to the batch processing API, featuring a complete redesign to implement a chained processor design. It reimagines how processors connect, allowing them to be linked together seamlessly in a processing pipeline. The redesign introduces new Filter and Transform processors, enhanced error handling, and better context cancellation support throughout the library. The PipelineStage has been replaced with more explicit interfaces to facilitate processor chaining, and the minimum Go version is updated to 1.18 to leverage generics.

### Added

- **Multi-processor support** enabling processor chaining in a single pipeline, a significant change to the interfaces.
  - Processors are executed in the order they're provided to `Batch.Go()`.
  - Each processor receives the output of the previous processor.
  - Core interfaces redesigned to facilitate this capability.
- New `Filter` processor for filtering items based on a predicate function.
  - Configurable with `Predicate` function to determine which items to keep.
  - `InvertMatch` option to invert filter logic (remove matching items instead of keeping them).
- New `Transform` processor for transforming item data.
  - Applies a transformation function to each item's `Data` field.
  - `ContinueOnError` option to control behavior when transformations fail.
  - Skips items that already have errors set.
- Improved source implementations with better error handling and context cancellation support.
  - `Channel` source now supports `BufferSize` configuration.
  - `Error` source now supports `BufferSize` and filters out nil errors.
  - `Nil` source now properly handles zero/negative durations and uses timers correctly.
- Added comprehensive test coverage for all source and processor implementations.
- `Processor` interface now takes and returns `[]*Item`, enabling true batch-level processing and per-item error tracking.
  - `Processor.Process` should be synchronous and must return only when processing is fully complete.
- `Source` interface updated to `Read(ctx) (<-chan interface{}, <-chan error)` to simplify usage and clarify ownership of channels.
  - `Source.Read` should spawn a goroutine and must close both output and error channels when finished.
- `Item` struct includes a new `Error error` field for capturing processor-level failures at item granularity.
- `waitAll(batch, errs)` helper function to await both completion (`Done()`) and error stream drain (`errs`).
- Full test coverage for all built-in sources: `Channel`, `Error`, `Nil`.
- New tests for processor chaining and individual error propagation.

### Changed

- Enhanced documentation across all interfaces and implementations.
- All processor implementations updated to follow the error pattern consistently.
- Source implementations now gracefully handle nil input channels.
- Better context cancellation handling in all source and processor implementations.
- Minimum supported Go version updated to **1.18**, enabling use of generics and improved concurrency patterns.
- Removed `PipelineStage`, replacing it with explicit slice and channel-based interfaces.
- `doReader` and `doProcessors` rewritten to use new interfaces with clear responsibility.
- Errors set by processors on individual items are now reported through `errs` as `ProcessorError`.

### Fixed

- Fixed linter errors and improved code quality throughout.
- Fixed potential deadlocks in source implementations.
- Fixed the processor.go file which had invalid syntax.
- All sources now properly respect context cancellation.
- Resolved potential deadlock when reading `errs` and awaiting `Done()` by introducing coordinated draining in tests and examples.

### Known Issues

- When a source is exhausted, items remaining in the pipeline will not be processed if their count is less than MinItems. This issue has been fixed in version 0.2.1.

## [0.1.1] - 2024-07-18

### Changed

- Improved README.md and added more detailed example.

## [0.1.0] - 2021-01-29

This is the initial release of GoBatch, a flexible and efficient batch processing library for Go.

### Added

- Core `Batch` structure for managing batch processing pipeline.
- `Source` interface for defining data input sources.
- `Processor` interface for implementing batch processing logic.
- `PipelineStage` struct for facilitating data flow between pipeline stages.
- `Item` struct with unique ID for tracking individual items through the pipeline.
- Configurable batch processing with `Config` interface and `ConfigValues` struct.
  - Minimum and maximum items per batch.
  - Minimum and maximum time to wait before processing a batch.
- `ConstantConfig` for static configuration.
- Basic error handling and reporting through error channels.
- `NextItem` helper function for implementing `Source.Read`.
- `IgnoreErrors` utility function for discarding errors.
- Comprehensive test suite for core functionality.
- Example implementations in `example_test.go`.

### Notes

- This version originally targeted Go 1.7 (later increased to 1.18).
- The library is in its early stages and the API may change significantly in future versions.
