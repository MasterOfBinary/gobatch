# Changelog

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

Note: This project is in early development. The API may change without warning in any 0.x version.

All notable changes to this project will be documented in this file.

## [0.2.0] - 2025-04-24

This release brings major improvements to the batch processing API, including a complete redesign of the pipeline architecture. It introduces new Filter and Transform processors, enhanced error handling, and better context cancellation support throughout the library. The PipelineStage has been replaced with more explicit interfaces, and the minimum Go version is updated to 1.18 to leverage generics.

### Added

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

- This version supports Go 1.7 currently but that may be increased later.
- The library is in its early stages and the API may change significantly in future versions.
