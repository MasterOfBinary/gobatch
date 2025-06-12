package batch_test

import (
	"sync"
	"testing"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
)

func TestNoOpStatsCollector(t *testing.T) {
	stats := &batch.NoOpStatsCollector{}

	// These should not panic
	stats.RecordBatchStart(10)
	stats.RecordBatchComplete(10, time.Second)
	stats.RecordItemProcessed()
	stats.RecordItemError()
	stats.RecordSourceError()
	stats.RecordProcessorError()

	// GetStats should return zero values
	s := stats.GetStats()
	if s.BatchesStarted != 0 || s.ItemsProcessed != 0 {
		t.Error("NoOpStatsCollector returned non-zero stats")
	}
}

func TestBasicStatsCollector(t *testing.T) {
	stats := batch.NewBasicStatsCollector()

	// Record some batches
	stats.RecordBatchStart(5)
	stats.RecordBatchComplete(5, 100*time.Millisecond)

	stats.RecordBatchStart(3)
	stats.RecordBatchComplete(3, 50*time.Millisecond)

	stats.RecordBatchStart(7)
	stats.RecordBatchComplete(7, 150*time.Millisecond)

	// Record items
	for i := 0; i < 10; i++ {
		stats.RecordItemProcessed()
	}
	for i := 0; i < 5; i++ {
		stats.RecordItemError()
	}

	// Record errors
	stats.RecordSourceError()
	stats.RecordSourceError()
	stats.RecordProcessorError()

	// Get stats
	s := stats.GetStats()

	// Verify counts
	if s.BatchesStarted != 3 {
		t.Errorf("BatchesStarted = %d, want 3", s.BatchesStarted)
	}
	if s.BatchesCompleted != 3 {
		t.Errorf("BatchesCompleted = %d, want 3", s.BatchesCompleted)
	}
	if s.ItemsProcessed != 10 {
		t.Errorf("ItemsProcessed = %d, want 10", s.ItemsProcessed)
	}
	if s.ItemErrors != 5 {
		t.Errorf("ItemErrors = %d, want 5", s.ItemErrors)
	}
	if s.SourceErrors != 2 {
		t.Errorf("SourceErrors = %d, want 2", s.SourceErrors)
	}
	if s.ProcessorErrors != 1 {
		t.Errorf("ProcessorErrors = %d, want 1", s.ProcessorErrors)
	}

	// Verify timing
	if s.MinBatchTime != 50*time.Millisecond {
		t.Errorf("MinBatchTime = %v, want 50ms", s.MinBatchTime)
	}
	if s.MaxBatchTime != 150*time.Millisecond {
		t.Errorf("MaxBatchTime = %v, want 150ms", s.MaxBatchTime)
	}
	if s.TotalProcessingTime != 300*time.Millisecond {
		t.Errorf("TotalProcessingTime = %v, want 300ms", s.TotalProcessingTime)
	}

	// Verify sizes
	if s.MinBatchSize != 3 {
		t.Errorf("MinBatchSize = %d, want 3", s.MinBatchSize)
	}
	if s.MaxBatchSize != 7 {
		t.Errorf("MaxBatchSize = %d, want 7", s.MaxBatchSize)
	}
}

func TestBasicStatsCollector_Concurrent(t *testing.T) {
	stats := batch.NewBasicStatsCollector()

	var wg sync.WaitGroup
	const goroutines = 10
	const operations = 100

	// Concurrently record various stats
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				stats.RecordBatchStart(id + j)
				stats.RecordItemProcessed()
				stats.RecordBatchComplete(id+j, time.Duration(j)*time.Millisecond)
				if j%5 == 0 {
					stats.RecordItemError()
				}
				if j%10 == 0 {
					stats.RecordSourceError()
				}
				if j%15 == 0 {
					stats.RecordProcessorError()
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify final counts
	s := stats.GetStats()
	expectedBatches := uint64(goroutines * operations)
	if s.BatchesStarted != expectedBatches {
		t.Errorf("BatchesStarted = %d, want %d", s.BatchesStarted, expectedBatches)
	}
	if s.BatchesCompleted != expectedBatches {
		t.Errorf("BatchesCompleted = %d, want %d", s.BatchesCompleted, expectedBatches)
	}
	if s.ItemsProcessed != expectedBatches {
		t.Errorf("ItemsProcessed = %d, want %d", s.ItemsProcessed, expectedBatches)
	}
}

func TestStats_CalculatedMetrics(t *testing.T) {
	tests := []struct {
		name         string
		stats        batch.Stats
		avgBatchTime time.Duration
		avgBatchSize float64
		errorRate    float64
	}{
		{
			name: "normal stats",
			stats: batch.Stats{
				BatchesCompleted:    5,
				ItemsProcessed:      45,
				ItemErrors:          5,
				TotalProcessingTime: 500 * time.Millisecond,
			},
			avgBatchTime: 100 * time.Millisecond,
			avgBatchSize: 9.0,
			errorRate:    10.0,
		},
		{
			name:         "no batches completed",
			stats:        batch.Stats{},
			avgBatchTime: 0,
			avgBatchSize: 0,
			errorRate:    0,
		},
		{
			name: "all errors",
			stats: batch.Stats{
				ItemErrors: 10,
			},
			errorRate: 100.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.stats.AverageBatchTime(); got != tt.avgBatchTime {
				t.Errorf("AverageBatchTime() = %v, want %v", got, tt.avgBatchTime)
			}
			if got := tt.stats.AverageBatchSize(); got != tt.avgBatchSize {
				t.Errorf("AverageBatchSize() = %v, want %v", got, tt.avgBatchSize)
			}
			if got := tt.stats.ErrorRate(); got != tt.errorRate {
				t.Errorf("ErrorRate() = %v, want %v", got, tt.errorRate)
			}
		})
	}
}

func TestStats_Duration(t *testing.T) {
	startTime := time.Now()
	stats := batch.Stats{
		StartTime:      startTime,
		LastUpdateTime: startTime.Add(5 * time.Second),
	}

	duration := stats.Duration()
	if duration != 5*time.Second {
		t.Errorf("Duration() = %v, want 5s", duration)
	}
}
