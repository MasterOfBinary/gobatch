# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

Note: This project is in early development. The API may change without warning in any 0.x version.

## [Unreleased]

### Changed

- Introduced golang generics support for entire library Batch, Source, and Processor interfaces
- Increased Go version requirement to 1.18

## [0.1.1] - 2024-07-18

### Changed

- Improved README.md and added more detailed example

## [0.1.0] - 2021-01-29

This is the initial release of GoBatch, a flexible and efficient batch processing library for Go.

### Added

- Core `Batch` structure for managing batch processing pipeline
- `Source` interface for defining data input sources
- `Processor` interface for implementing batch processing logic
- `PipelineStage` struct for facilitating data flow between pipeline stages
- `Item` struct with unique ID for tracking individual items through the pipeline
- Configurable batch processing with `Config` interface and `ConfigValues` struct
    - Minimum and maximum items per batch
    - Minimum and maximum time to wait before processing a batch
- `ConstantConfig` for static configuration
- Basic error handling and reporting through error channels
- `NextItem` helper function for implementing `Source.Read`
- `IgnoreErrors` utility function for discarding errors
- Comprehensive test suite for core functionality
- Example implementations in `example_test.go`

### Notes

- This version supports Go 1.7 currently but that may be increased later
- The library is in its early stages and the API may change significantly in future versions