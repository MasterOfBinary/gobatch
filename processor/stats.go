package processor

import (
	"context"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
)

// StatsProcessor wraps another processor and collects statistics about its execution.
// It tracks processing times, item counts, and error rates for the wrapped processor.
type StatsProcessor struct {
	// Processor is the wrapped processor that does the actual work.
	Processor batch.Processor

	// Stats is used to collect processing metrics.
	// If nil, no statistics are collected.
	Stats batch.StatsCollector

	// RecordAsBatch determines whether to record statistics as batch-level metrics.
	// If true, uses RecordBatchStart/Complete. If false, only records item-level metrics.
	RecordAsBatch bool
}

// Process implements the Processor interface by delegating to the wrapped processor
// and collecting statistics about the operation.
func (p *StatsProcessor) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	if p.Processor == nil {
		return items, nil
	}

	if p.Stats == nil {
		// No stats collector, just pass through
		return p.Processor.Process(ctx, items)
	}

	startTime := time.Now()

	if p.RecordAsBatch {
		p.Stats.RecordBatchStart(len(items))
	}

	result, err := p.Processor.Process(ctx, items)

	// Record item-level stats
	for _, item := range result {
		if item.Error != nil {
			p.Stats.RecordItemError()
		} else {
			p.Stats.RecordItemProcessed()
		}
	}

	// Record processor error if any
	if err != nil {
		p.Stats.RecordProcessorError()
	}

	if p.RecordAsBatch {
		duration := time.Since(startTime)
		p.Stats.RecordBatchComplete(len(result), duration)
	}

	return result, err
}

// WrapWithStats wraps a processor with statistics collection.
// This is a convenience function for creating a StatsProcessor.
//
// Example:
//
//	stats := batch.NewBasicStatsCollector()
//	wrapped := processor.WrapWithStats(myProcessor, stats, false)
//
//	// Later, get statistics
//	currentStats := stats.GetStats()
func WrapWithStats(proc batch.Processor, stats batch.StatsCollector, recordAsBatch bool) *StatsProcessor {
	return &StatsProcessor{
		Processor:     proc,
		Stats:         stats,
		RecordAsBatch: recordAsBatch,
	}
}
