# Architecture

GoBatch follows a **pipeline-based batch processing architecture** with five distinct layers:

1. **Source Layer** (`source/` package): Data ingestion abstraction. Implementations include Channel, Error, and Nil sources that return item and error channels.

2. **Batching/Orchestration Layer** (`batch/batch.go`): Core Batch type coordinates three goroutines - doIDGenerator (unique ID assignment), doReader (wraps source items with IDs), doProcessors (batches items and chains processors).

3. **Configuration Layer** (`batch/config.go`): Config interface with ConstantConfig and DynamicConfig implementations control batching timing (MinTime/MaxTime) and sizing (MinItems/MaxItems) with priority: `MaxTime = MaxItems > EOF > MinTime > MinItems`.

4. **Processor Layer** (`processor/` package): Processing abstraction for batch transformations. Built-in processors: Transform (modify data), Filter (remove items), Channel (write to output), Error (simulate failures), Nil (passthrough).

5. **Helper/Utility Layer** (`batch/helpers.go`, `batch/errors.go`): Convenience functions (IgnoreErrors, CollectErrors, RunBatchAndWait, ExecuteBatches) and error types (SourceError, ProcessorError).

## Data Flow

Source.Read() → doReader wraps items with IDs → items accumulate → waitForItems determines batch ready based on Config → doProcessors batches spawn goroutines → each batch flows through Processor chain sequentially → errors wrapped and sent on error channel → Done signals completion.

## Key Abstractions

- **Source[T]**: Must return (items, errors) channels, close both, respect context
- **Processor[T]**: Takes batch, returns modified batch + error, respects context
- **Config**: Get() called per batch, supports dynamic runtime updates
- **Item[T]**: ID (immutable), Data (mutable), Error (settable)

## Entry Points

- `Batch.Go()`: Starts pipeline, returns error channel
- `Batch.Done()`: Returns channel that closes on completion
- Helpers provide simplified patterns (RunBatchAndWait combines Go/errors/Done)

---

*Architecture analysis: 2026-04-10*
