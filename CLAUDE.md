# Claude Instructions

This file contains instructions and guidelines for Claude when working with this project.

## Project Overview
GoBatch is a Go library for batch data processing. It provides infrastructure for processing batches of data with configurable sources, processors, and pipelines.

## Important Commands
- Run tests: `go test ./...`
- Lint code: `go vet ./...`

## Code Standards
- Follow Go best practices and idiomatic Go patterns
- Ensure all exported functions are properly documented
- Write tests for new functionality
- Maintain backward compatibility where possible

## File Structure
- `/batch`: Core batch processing functionality
- `/processor`: Different processors for data manipulation
- `/source`: Data sources for batch processing
- `/pipeline`: Pipeline functionality for chaining processors