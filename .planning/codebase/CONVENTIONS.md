# Conventions

## Code Style

- Standard `gofmt` formatting — no custom linter configuration
- Idiomatic Go patterns throughout
- CI runs `go vet ./...` and `golangci-lint run --timeout=3m`

## Naming

### Types
- PascalCase for exported types: `Batch[T]`, `Source[T]`, `Processor[T]`, `Item[T]`, `Config`, `ConstantConfig`, `DynamicConfig`
- Error types: `SourceError`, `ProcessorError`, `ItemError`
- Generics use brackets: `[T any]`

### Functions
- PascalCase for exported: `New[T]()`, `Go()`, `Done()`, `IgnoreErrors()`, `CollectErrors()`
- camelCase for unexported: `doReader()`, `doProcessors()`, `waitForItems()`, `fixConfig()`

### Files
- Lowercase, single-word names: `batch.go`, `config.go`, `errors.go`, `helpers.go`
- Implementation files named after primary type: `transform.go`, `filter.go`, `channel.go`
- Test files: `{name}_test.go`
- Package docs: `doc.go` in each package

## Patterns

### Constructor Pattern
- `New[T]()` constructors for processor and source types
- Accept functional dependencies (functions, channels) as constructor params
- Example: `processor.NewTransform[T](func(data T) (T, error))` in `processor/transform.go`

### Interface Design
- Small, focused interfaces: `Source[T]` (Read), `Processor[T]` (Process), `Config` (Get)
- Generic type parameters on interfaces for type safety
- Interfaces defined in `batch/batch.go` alongside the core Batch type

### Concurrency
- `sync.Mutex` for protecting shared state (Batch.mu)
- `sync.WaitGroup` for goroutine coordination
- `sync/atomic` for the per-batch ID counter (`Batch.nextID`)
- Channel-based communication between pipeline stages
- `context.Context` for cancellation propagation; `waitForItems` watches `ctx.Done()`
- Goroutines spawned in `Go()`: doReader, doProcessors

### Configuration
- `Config` interface with `Get()` method returning `ConfigValues`
- `ConstantConfig`: immutable, set at creation
- `DynamicConfig`: runtime-updatable via `Update()` with mutex protection
- `BufferConfig`: controls internal channel buffer sizes

## Error Handling

### Custom Error Types
- `SourceError` wraps errors returned by `Source.Read()` — defined in `batch/errors.go`
- `ProcessorError` wraps processor-wide errors returned as the second value of `Processor.Process()` — defined in `batch/errors.go`
- `ItemError` wraps per-item failures (`item.Error`) and carries the failing `ItemID` — defined in `batch/errors.go`
- All three implement `Unwrap()` for `errors.As()` inspection

### Error Propagation
- Errors sent on dedicated error channel returned by `Batch.Go()`
- Item-level errors tracked via `Item.Error` field
- Processors should check and respect existing item errors
- Processing continues despite individual item errors

### Error Utilities
- `IgnoreErrors()`: drains error channel in background goroutine
- `CollectErrors()`: collects all errors into slice after completion
- `RunBatchAndWait()`: combines Go/errors/Done into single call

## Documentation Style

- Godoc comments on all exported types, functions, methods, constants
- Package-level `doc.go` in each package with overview and examples
- Interface documentation includes usage examples
- Method docs explain parameters, return values, and behavior

---

*Conventions analysis: 2026-04-10. Updated 2026-04-29 for the cancellation-and-IDs refactor (atomic counter, ItemError type, ctx-aware waitForItems).*
