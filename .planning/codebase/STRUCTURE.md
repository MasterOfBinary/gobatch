# Structure

## Directory Layout

```
/Users/vaughn/dev/gobatch/
├── batch/              # Core batching engine
│   ├── batch.go        # Batch type, Source/Processor interfaces
│   ├── config.go       # Config interface, ConstantConfig, DynamicConfig
│   ├── errors.go       # SourceError, ProcessorError types
│   ├── helpers.go      # IgnoreErrors, CollectErrors, RunBatchAndWait, ExecuteBatches
│   ├── constants.go    # Default buffer sizes
│   ├── doc.go          # Package documentation
│   └── *_test.go       # Tests (50+ test files covering all functionality)
├── processor/          # Processor implementations
│   ├── transform.go    # Transform processor (modify data)
│   ├── filter.go       # Filter processor (remove items)
│   ├── channel.go      # Channel processor (write to output)
│   ├── error.go        # Error processor (simulate failures)
│   ├── nil.go          # Nil processor (passthrough)
│   ├── doc.go          # Package documentation
│   └── *_test.go       # Tests
├── source/             # Source implementations
│   ├── channel.go      # Channel source
│   ├── error.go        # Error source
│   ├── nil.go          # Nil source
│   ├── doc.go          # Package documentation
│   └── *_test.go       # Tests
├── doc.go              # Root package documentation
├── example_test.go     # Top-level usage examples
└── .planning/
    └── codebase/       # Documentation (this directory)
```

## Key File Locations

### Entry Points
- `batch/batch.go`: Main Batch type, Go() and Done() methods
- `example_test.go`: Complete usage example showing source → processor pipeline

### Configuration
- `batch/config.go`: Config interface (Get method), ConstantConfig (static), DynamicConfig (runtime-updatable)

### Core Logic
- `batch/batch.go` — `Go()` starts the pipeline (assigns IDs via an atomic counter, spawns `doReader` and `doProcessors`)
- `batch/batch.go` — `waitForItems` implements batching strategy and honors `ctx.Done()` for cancellation drain
- `batch/batch.go` — `doProcessors` runs the processor chain and emits `*ItemError` for per-item failures, `*ProcessorError` for processor-wide failures

### Interfaces
- `batch/batch.go` — `Source[T]`: `Read(ctx)` returns items and errors channels; both must be closed when the source observes ctx done
- `batch/batch.go` — `Processor[T]`: `Process(ctx, items)` returns modified items + error

### Error Handling
- `batch/errors.go`: `SourceError`, `ProcessorError`, `ItemError` (with `ItemID`), all implementing `Unwrap()` for error chaining

### Utilities
- `batch/helpers.go`: IgnoreErrors, CollectErrors, RunBatchAndWait, ExecuteBatches, BatchConfig
- `batch/constants.go`: Default buffer sizes

## Naming Conventions

### Files
- Source implementations: `source/{channel,error,nil}.go`
- Processor implementations: `processor/{transform,filter,channel,error,nil}.go`
- Tests: `{package}/{type}_test.go` or `example_test.go` for example-based tests

### Directories
- Package name matches directory: `batch/`, `processor/`, `source/`
- Each package has `doc.go` for godoc package documentation

### Functions
- Capitalized exported functions: `New[T]()`, `Go()`, `Done()`, `IgnoreErrors()`, `CollectErrors()`
- Capitalized exported methods: `Process()`, `Read()`, `Get()`, `Update()`
- Unexported goroutines: `doReader()`, `doProcessors()`

### Types
- Capitalized: `Batch[T]`, `Source[T]`, `Processor[T]`, `Item[T]`, `Config`, `ConstantConfig`, `DynamicConfig`
- Error types: `SourceError`, `ProcessorError`, `ItemError`

### Interfaces
- Named ending with capitalized letter: `Source[T]`, `Processor[T]`, `Config`
- Methods are capitalized: `Read()`, `Process()`, `Get()`

## Where to Add New Code

### New Processor Implementation
- Location: `processor/{name}.go`
- Must implement: `Process(ctx context.Context, items []*batch.Item[T]) ([]*batch.Item[T], error)`
- Follow Transform/Filter pattern: check item.Error, modify items, set item.Error or return error
- Add tests: `processor/{name}_test.go`
- Document: Add godoc comment to type and Process method

### New Source Implementation
- Location: `source/{name}.go`
- Must implement: `Read(ctx context.Context) (<-chan T, <-chan error)`
- Must close both channels when done
- Respect context cancellation in select statements
- Add tests: `source/{name}_test.go`
- Document: Add godoc comment to type and Read method

### New Helper Function
- Location: `batch/helpers.go`
- Signature should follow existing patterns (accept Batch, Source, Processor, return []error)
- Add tests: `batch/{name}_test.go`
- Document with godoc including example usage

### Tests
- Co-located next to implementation (same directory)
- Naming: `{file}_test.go` or `example_{type}_test.go` for examples
- Table-driven tests preferred for multiple scenarios

## Special Directories

### batch/
- Purpose: Core batching engine and orchestration
- Key invariant: Batch type must be thread-safe with mutex protection for state changes

### processor/
- Purpose: Built-in data transformation processors
- Key invariant: All processors must respect context cancellation

### source/
- Purpose: Built-in data source implementations
- Key invariant: All sources must close both channels they return

---

*Structure analysis: 2026-04-10. Updated 2026-04-29 for the cancellation-and-IDs refactor (no more `doIDGenerator` goroutine, atomic ID counter, `ItemError` type).*
