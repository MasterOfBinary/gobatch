package processor

import (
	"context"
	"fmt"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
)

// LoggingProcessor wraps another processor and adds logging capabilities.
// It logs when processing starts and completes, along with any errors encountered.
type LoggingProcessor struct {
	// Processor is the wrapped processor that does the actual work.
	Processor batch.Processor

	// Logger is used to log processing events.
	// If nil, no logging occurs.
	Logger batch.Logger

	// Name is an optional name for this processor used in log messages.
	// If empty, a generic name is used.
	Name string
}

// Process implements the Processor interface by delegating to the wrapped processor
// and logging the operation.
func (p *LoggingProcessor) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	if p.Processor == nil {
		return items, nil
	}

	if p.Logger == nil {
		// No logger, just pass through
		return p.Processor.Process(ctx, items)
	}

	name := p.Name
	if name == "" {
		name = fmt.Sprintf("%T", p.Processor)
	}

	startTime := time.Now()
	p.Logger.Debug("Processor '%s' starting with %d items", name, len(items))

	result, err := p.Processor.Process(ctx, items)

	duration := time.Since(startTime)
	if err != nil {
		p.Logger.Error("Processor '%s' failed after %v: %v", name, duration, err)
	} else {
		// Count successes and errors
		var successCount, errorCount int
		for _, item := range result {
			if item.Error != nil {
				errorCount++
			} else {
				successCount++
			}
		}
		p.Logger.Debug("Processor '%s' completed in %v: %d items (%d successful, %d errors)",
			name, duration, len(result), successCount, errorCount)
	}

	return result, err
}

// WrapWithLogging wraps a processor with logging capabilities.
// This is a convenience function for creating a LoggingProcessor.
//
// Example:
//
//	logger := batch.NewSimpleLogger(batch.LogLevelDebug)
//	wrapped := processor.WrapWithLogging(myProcessor, logger, "MyProcessor")
func WrapWithLogging(proc batch.Processor, logger batch.Logger, name string) *LoggingProcessor {
	return &LoggingProcessor{
		Processor: proc,
		Logger:    logger,
		Name:      name,
	}
}
