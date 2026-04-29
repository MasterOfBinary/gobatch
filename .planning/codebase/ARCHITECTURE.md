# Architecture

GoBatch follows a **pipeline-based batch processing architecture** with five distinct layers:

1. **Source Layer** (`source/` package): Data ingestion abstraction. Implementations include Channel, Error, and Nil sources that return item and error channels.

2. **Batching/Orchestration Layer** (`batch/batch.go`): Core Batch type coordinates two goroutines - doReader (assigns each item a unique ID via an atomic counter and forwards it on the items channel) and doProcessors (collects items into batches and chains them through Processors). Cancellation flows from `ctx` into `waitForItems`, which returns any partial batch immediately and then drains remaining buffered items one at a time so already-read items are never dropped.

3. **Configuration Layer** (`batch/config.go`): Config interface with ConstantConfig and DynamicConfig implementations control batching timing (MinTime/MaxTime) and sizing (MinItems/MaxItems) with priority: `MaxTime = MaxItems > EOF > MinTime > MinItems`.

4. **Processor Layer** (`processor/` package): Processing abstraction for batch transformations. Built-in processors: Transform (modify data), Filter (remove items), Channel (write to output), Error (simulate failures), Nil (passthrough).

5. **Helper/Utility Layer** (`batch/helpers.go`, `batch/errors.go`): Convenience functions (IgnoreErrors, CollectErrors, RunBatchAndWait, ExecuteBatches) and error types (SourceError, ProcessorError, ItemError). ItemError carries the failing `ItemID` so callers can correlate per-item failures with specific items, while ProcessorError remains for processor-wide failures.

## Data Flow

Source.Read() → doReader assigns IDs via `atomic.AddUint64` and forwards items → items accumulate in `b.items` → waitForItems determines batch ready based on Config (or returns early on `ctx.Done()`) → doProcessors batches spawn goroutines → each batch flows through Processor chain sequentially → errors wrapped (`SourceError` / `ProcessorError` / `ItemError`) and sent on error channel → Done signals completion.

## Key Abstractions

- **Source[T]**: Must return (items, errors) channels, close both, respect context
- **Processor[T]**: Takes batch, returns modified batch + error, respects context
- **Config**: Get() called per batch, supports dynamic runtime updates
- **Item[T]**: ID (immutable), Data (mutable), Error (settable)

## Entry Points

- `Batch.Go()`: Starts pipeline, returns error channel
- `Batch.Done()`: Returns channel that closes on completion
- Helpers provide simplified patterns (RunBatchAndWait combines Go/errors/Done)

## Cancellation Contract

`waitForItems` honors `ctx.Done()` for early shutdown, but it ultimately blocks until `b.items` closes. `b.items` is closed by `doReader` only after the source closes both of its channels, so cancellation only takes effect once the source observes ctx. **Sources that ignore ctx will block the pipeline indefinitely after cancel.** Source authors must propagate ctx into their `Read` goroutine.

---

*Architecture analysis: 2026-04-10. Updated 2026-04-29 to reflect the atomic-counter ID generator and ctx-aware waitForItems.*
