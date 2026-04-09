# Testing

## Framework

- Go's built-in `testing` package — no external test frameworks
- Race detection enabled: `go test -race ./...`
- Coverage profiling: `go test -coverprofile=coverage.txt -covermode=atomic ./...`

## Test Organization

- Tests co-located with source files in same package
- Naming: `{file}_test.go` (e.g., `batch_test.go`, `config_test.go`, `transform_test.go`)
- Example tests: `example_test.go` at root and `example_{type}_test.go` in packages
- Test helpers: `testhelpers_test.go` in `batch/` package

## Test Helpers

Defined in `batch/testhelpers_test.go`:
- `testSource`: configurable test source that emits items on a channel
- `countProcessor`: counts items processed (for verification)
- `errorPerItemProcessor`: sets error on each item (for error path testing)
- Helper functions for creating test configurations and batch setups

## Test Patterns

### Table-Driven Tests
- Used for testing multiple scenarios with `t.Run()` subtests
- Common in config tests (`batch/config_test.go`) and processor tests

### Concurrency Testing
- Race detection (`-race` flag) catches data races
- Tests verify goroutine cleanup and channel closure
- Context cancellation tests verify proper shutdown

### Error Path Testing
- Dedicated error sources (`source.NewError[T]()`) and processors (`processor.NewError[T]()`)
- Tests verify error wrapping with `errors.As()` for SourceError/ProcessorError
- Item-level error propagation tested

### Example Tests
- `example_test.go`: end-to-end pipeline examples using `Example` functions
- Serve as both documentation and regression tests
- Verified by `go test` (output checked against `// Output:` comments)

## Coverage

- CI runs with `-covermode=atomic` for accurate coverage with goroutines
- Coverage uploaded to Codecov via GitHub Actions (`.github/workflows/ci.yml`)

## Test Commands

```bash
# Run all tests with race detection and coverage
go test -race -coverprofile=coverage.txt -covermode=atomic ./...

# Run specific package tests
go test -race ./batch/...
go test -race ./processor/...
go test -race ./source/...

# Run specific test
go test -race -run TestBatchGo ./batch/...
```

---

*Testing analysis: 2026-04-10*
