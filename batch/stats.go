package batch

import (
	"sync"
	"sync/atomic"
	"time"
)

// StatsCollector defines the interface for collecting metrics during batch processing.
// Implementations can store metrics in memory, send to monitoring systems, or export to various formats.
// The StatsCollector is optional - if not provided, no statistics are collected.
type StatsCollector interface {
	// RecordBatchStart is called when a batch starts processing.
	RecordBatchStart(batchSize int)

	// RecordBatchComplete is called when a batch completes processing.
	// duration is the time taken to process the batch.
	RecordBatchComplete(batchSize int, duration time.Duration)

	// RecordItemProcessed is called for each successfully processed item.
	RecordItemProcessed()

	// RecordItemError is called when an item encounters an error during processing.
	RecordItemError()

	// RecordSourceError is called when the source encounters an error.
	RecordSourceError()

	// RecordProcessorError is called when a processor encounters an error.
	RecordProcessorError()

	// GetStats returns a snapshot of the current statistics.
	GetStats() Stats
}

// Stats holds aggregated statistics about batch processing.
type Stats struct {
	// BatchesStarted is the total number of batches that have started processing.
	BatchesStarted uint64

	// BatchesCompleted is the total number of batches that have completed processing.
	BatchesCompleted uint64

	// ItemsProcessed is the total number of items successfully processed.
	ItemsProcessed uint64

	// ItemErrors is the total number of items that encountered errors.
	ItemErrors uint64

	// SourceErrors is the total number of errors from sources.
	SourceErrors uint64

	// ProcessorErrors is the total number of errors from processors.
	ProcessorErrors uint64

	// TotalProcessingTime is the cumulative time spent processing all batches.
	TotalProcessingTime time.Duration

	// MinBatchTime is the minimum time taken to process a batch.
	MinBatchTime time.Duration

	// MaxBatchTime is the maximum time taken to process a batch.
	MaxBatchTime time.Duration

	// MinBatchSize is the smallest batch size processed.
	MinBatchSize int

	// MaxBatchSize is the largest batch size processed.
	MaxBatchSize int

	// StartTime is when statistics collection began.
	StartTime time.Time

	// LastUpdateTime is when statistics were last updated.
	LastUpdateTime time.Time
}

// NoOpStatsCollector is a stats collector that discards all metrics.
// It implements the StatsCollector interface but performs no operations.
// This is the default stats collector when none is specified.
type NoOpStatsCollector struct{}

// RecordBatchStart implements the StatsCollector interface.
func (n *NoOpStatsCollector) RecordBatchStart(batchSize int) {}

// RecordBatchComplete implements the StatsCollector interface.
func (n *NoOpStatsCollector) RecordBatchComplete(batchSize int, duration time.Duration) {}

// RecordItemProcessed implements the StatsCollector interface.
func (n *NoOpStatsCollector) RecordItemProcessed() {}

// RecordItemError implements the StatsCollector interface.
func (n *NoOpStatsCollector) RecordItemError() {}

// RecordSourceError implements the StatsCollector interface.
func (n *NoOpStatsCollector) RecordSourceError() {}

// RecordProcessorError implements the StatsCollector interface.
func (n *NoOpStatsCollector) RecordProcessorError() {}

// GetStats implements the StatsCollector interface.
func (n *NoOpStatsCollector) GetStats() Stats {
	return Stats{}
}

// BasicStatsCollector is a simple in-memory implementation of StatsCollector.
// It maintains counters and timing information about batch processing.
// All operations are thread-safe.
type BasicStatsCollector struct {
	mu    sync.RWMutex
	stats Stats

	// Atomic counters for lock-free updates
	batchesStarted   uint64
	batchesCompleted uint64
	itemsProcessed   uint64
	itemErrors       uint64
	sourceErrors     uint64
	processorErrors  uint64
}

// NewBasicStatsCollector creates a new BasicStatsCollector.
func NewBasicStatsCollector() *BasicStatsCollector {
	return &BasicStatsCollector{
		stats: Stats{
			StartTime:      time.Now(),
			LastUpdateTime: time.Now(),
			MinBatchTime:   time.Duration(1<<63 - 1), // Max duration as initial value
		},
	}
}

// RecordBatchStart implements the StatsCollector interface.
func (b *BasicStatsCollector) RecordBatchStart(batchSize int) {
	atomic.AddUint64(&b.batchesStarted, 1)

	b.mu.Lock()
	defer b.mu.Unlock()

	b.stats.LastUpdateTime = time.Now()

	if batchSize < b.stats.MinBatchSize || b.stats.MinBatchSize == 0 {
		b.stats.MinBatchSize = batchSize
	}
	if batchSize > b.stats.MaxBatchSize {
		b.stats.MaxBatchSize = batchSize
	}
}

// RecordBatchComplete implements the StatsCollector interface.
func (b *BasicStatsCollector) RecordBatchComplete(batchSize int, duration time.Duration) {
	atomic.AddUint64(&b.batchesCompleted, 1)

	b.mu.Lock()
	defer b.mu.Unlock()

	b.stats.LastUpdateTime = time.Now()
	b.stats.TotalProcessingTime += duration

	if duration < b.stats.MinBatchTime {
		b.stats.MinBatchTime = duration
	}
	if duration > b.stats.MaxBatchTime {
		b.stats.MaxBatchTime = duration
	}
}

// RecordItemProcessed implements the StatsCollector interface.
func (b *BasicStatsCollector) RecordItemProcessed() {
	atomic.AddUint64(&b.itemsProcessed, 1)
}

// RecordItemError implements the StatsCollector interface.
func (b *BasicStatsCollector) RecordItemError() {
	atomic.AddUint64(&b.itemErrors, 1)
}

// RecordSourceError implements the StatsCollector interface.
func (b *BasicStatsCollector) RecordSourceError() {
	atomic.AddUint64(&b.sourceErrors, 1)
}

// RecordProcessorError implements the StatsCollector interface.
func (b *BasicStatsCollector) RecordProcessorError() {
	atomic.AddUint64(&b.processorErrors, 1)
}

// GetStats implements the StatsCollector interface.
// It returns a snapshot of the current statistics.
func (b *BasicStatsCollector) GetStats() Stats {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Copy the stats and update atomic values
	stats := b.stats
	stats.BatchesStarted = atomic.LoadUint64(&b.batchesStarted)
	stats.BatchesCompleted = atomic.LoadUint64(&b.batchesCompleted)
	stats.ItemsProcessed = atomic.LoadUint64(&b.itemsProcessed)
	stats.ItemErrors = atomic.LoadUint64(&b.itemErrors)
	stats.SourceErrors = atomic.LoadUint64(&b.sourceErrors)
	stats.ProcessorErrors = atomic.LoadUint64(&b.processorErrors)

	// Fix min batch time if no batches completed
	if stats.BatchesCompleted == 0 {
		stats.MinBatchTime = 0
	}

	return stats
}

// AverageBatchTime returns the average time taken to process a batch.
// Returns 0 if no batches have been completed.
func (s *Stats) AverageBatchTime() time.Duration {
	if s.BatchesCompleted == 0 {
		return 0
	}
	return s.TotalProcessingTime / time.Duration(s.BatchesCompleted)
}

// AverageBatchSize returns the average size of processed batches.
// Returns 0 if no batches have been completed.
func (s *Stats) AverageBatchSize() float64 {
	if s.BatchesCompleted == 0 {
		return 0
	}
	return float64(s.ItemsProcessed) / float64(s.BatchesCompleted)
}

// ErrorRate returns the percentage of items that encountered errors.
// Returns 0 if no items have been processed.
func (s *Stats) ErrorRate() float64 {
	total := s.ItemsProcessed + s.ItemErrors
	if total == 0 {
		return 0
	}
	return float64(s.ItemErrors) / float64(total) * 100
}

// Duration returns the total duration since statistics collection started.
func (s *Stats) Duration() time.Duration {
	return s.LastUpdateTime.Sub(s.StartTime)
}
