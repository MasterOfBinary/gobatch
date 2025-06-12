# Logging and Statistics in GoBatch

This document describes the logging and statistics capabilities added to GoBatch.

## Overview

GoBatch now includes optional logging and statistics collection features that allow you to:
- Monitor batch processing in real-time with configurable logging
- Collect detailed metrics about processing performance
- Track errors and success rates
- Measure processing times and throughput

## Logging

### Logger Interface

The `Logger` interface provides structured logging with multiple log levels:

```go
type Logger interface {
    Log(level LogLevel, format string, args ...interface{})
    Debug(format string, args ...interface{})
    Info(format string, args ...interface{})
    Warn(format string, args ...interface{})
    Error(format string, args ...interface{})
}
```

### Built-in Implementations

1. **NoOpLogger**: Default logger that discards all messages (zero overhead when logging is not needed)
2. **SimpleLogger**: Basic logger that writes to stdout/stderr with timestamps

### Usage

```go
// Create a logger
logger := batch.NewSimpleLogger(batch.LogLevelInfo)

// Attach to batch
b := batch.New(config).WithLogger(logger)

// The batch will now log:
// - Processing start/completion
// - Source reading progress
// - Batch processing details
// - Errors as they occur
```

### Processor Logging

You can also wrap individual processors with logging:

```go
// Wrap a processor with logging
wrapped := processor.WrapWithLogging(myProcessor, logger, "MyProcessor")
```

## Statistics Collection

### StatsCollector Interface

The `StatsCollector` interface tracks various metrics:

```go
type StatsCollector interface {
    RecordBatchStart(batchSize int)
    RecordBatchComplete(batchSize int, duration time.Duration)
    RecordItemProcessed()
    RecordItemError()
    RecordSourceError()
    RecordProcessorError()
    GetStats() Stats
}
```

### Metrics Tracked

The `Stats` struct provides:
- Batch counts (started, completed)
- Item counts (processed, errors)
- Error counts by type (source, processor)
- Timing information (min, max, average, total)
- Batch sizes (min, max, average)
- Error rates

### Built-in Implementations

1. **NoOpStatsCollector**: Default collector that does nothing (zero overhead)
2. **BasicStatsCollector**: Thread-safe in-memory statistics collector

### Usage

```go
// Create a stats collector
stats := batch.NewBasicStatsCollector()

// Attach to batch
b := batch.New(config).WithStats(stats)

// Process data...

// Retrieve statistics
finalStats := stats.GetStats()
fmt.Printf("Items processed: %d\n", finalStats.ItemsProcessed)
fmt.Printf("Error rate: %.1f%%\n", finalStats.ErrorRate())
fmt.Printf("Average batch time: %v\n", finalStats.AverageBatchTime())
```

### Processor Statistics

Individual processors can also collect statistics:

```go
// Wrap a processor with stats collection
wrapped := processor.WrapWithStats(myProcessor, stats, true)
```

## Complete Example

```go
package main

import (
    "context"
    "fmt"
    "github.com/MasterOfBinary/gobatch/batch"
    "github.com/MasterOfBinary/gobatch/processor"
    "github.com/MasterOfBinary/gobatch/source"
)

func main() {
    // Setup logging and stats
    logger := batch.NewSimpleLogger(batch.LogLevelInfo)
    stats := batch.NewBasicStatsCollector()
    
    // Configure batch processing
    config := batch.NewConstantConfig(&batch.ConfigValues{
        MinItems: 10,
        MaxItems: 100,
        MaxTime:  5 * time.Second,
    })
    
    // Create batch with logging and stats
    b := batch.New(config).
        WithLogger(logger).
        WithStats(stats)
    
    // Create source and processor
    src := createSource()
    proc := createProcessor()
    
    // Run batch processing
    ctx := context.Background()
    errs := b.Go(ctx, src, proc)
    
    // Handle errors
    for err := range errs {
        fmt.Printf("Error: %v\n", err)
    }
    
    // Wait for completion
    <-b.Done()
    
    // Display final statistics
    finalStats := stats.GetStats()
    fmt.Printf("\n=== Final Statistics ===\n")
    fmt.Printf("Total batches: %d\n", finalStats.BatchesCompleted)
    fmt.Printf("Total items: %d processed, %d errors\n", 
        finalStats.ItemsProcessed, finalStats.ItemErrors)
    fmt.Printf("Error rate: %.1f%%\n", finalStats.ErrorRate())
    fmt.Printf("Average batch size: %.1f\n", finalStats.AverageBatchSize())
    fmt.Printf("Average processing time: %v\n", finalStats.AverageBatchTime())
}
```

## Performance Considerations

- Both logging and statistics are optional with zero overhead when not used
- The default NoOp implementations are used when not explicitly configured
- All statistics collection is thread-safe
- Logging can be filtered by level to reduce overhead
- Consider using LogLevelWarn or LogLevelError in production for minimal overhead

## Integration Notes

- Logging and statistics must be configured before calling `Go()`
- Both features are designed to be non-intrusive and maintain backward compatibility
- Custom implementations of Logger and StatsCollector interfaces can be provided
- The processor wrappers (LoggingProcessor, StatsProcessor) can be used independently