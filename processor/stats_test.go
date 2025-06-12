package processor_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/processor"
)

func TestStatsProcessor_Success(t *testing.T) {
	stats := batch.NewBasicStatsCollector()
	innerProc := &testProcessor{
		processFunc: func(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
			// Simulate some processing time
			time.Sleep(10 * time.Millisecond)
			return items, nil
		},
	}

	statsProc := &processor.StatsProcessor{
		Processor:     innerProc,
		Stats:         stats,
		RecordAsBatch: true,
	}

	items := []*batch.Item{
		{ID: 1, Data: "one"},
		{ID: 2, Data: "two"},
		{ID: 3, Data: "three"},
	}

	ctx := context.Background()
	result, err := statsProc.Process(ctx, items)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(result) != len(items) {
		t.Errorf("expected %d items, got %d", len(items), len(result))
	}

	// Check stats
	s := stats.GetStats()
	if s.BatchesStarted != 1 {
		t.Errorf("expected 1 batch started, got %d", s.BatchesStarted)
	}
	if s.BatchesCompleted != 1 {
		t.Errorf("expected 1 batch completed, got %d", s.BatchesCompleted)
	}
	if s.ItemsProcessed != 3 {
		t.Errorf("expected 3 items processed, got %d", s.ItemsProcessed)
	}
	if s.ItemErrors != 0 {
		t.Errorf("expected 0 item errors, got %d", s.ItemErrors)
	}
}

func TestStatsProcessor_ItemErrors(t *testing.T) {
	stats := batch.NewBasicStatsCollector()
	innerProc := &testProcessor{
		processFunc: func(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
			// Mark some items as failed
			items[0].Error = errors.New("error 1")
			items[2].Error = errors.New("error 2")
			return items, nil
		},
	}

	statsProc := &processor.StatsProcessor{
		Processor: innerProc,
		Stats:     stats,
	}

	items := []*batch.Item{
		{ID: 1, Data: "one"},
		{ID: 2, Data: "two"},
		{ID: 3, Data: "three"},
	}

	ctx := context.Background()
	_, err := statsProc.Process(ctx, items)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Check stats
	s := stats.GetStats()
	if s.ItemsProcessed != 1 {
		t.Errorf("expected 1 item processed, got %d", s.ItemsProcessed)
	}
	if s.ItemErrors != 2 {
		t.Errorf("expected 2 item errors, got %d", s.ItemErrors)
	}
}

func TestStatsProcessor_ProcessorError(t *testing.T) {
	stats := batch.NewBasicStatsCollector()
	procErr := errors.New("processor error")
	innerProc := &testProcessor{
		processFunc: func(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
			return items, procErr
		},
	}

	statsProc := &processor.StatsProcessor{
		Processor: innerProc,
		Stats:     stats,
	}

	items := []*batch.Item{{ID: 1, Data: "test"}}
	ctx := context.Background()
	_, err := statsProc.Process(ctx, items)

	if err != procErr {
		t.Errorf("expected error %v, got %v", procErr, err)
	}

	// Check stats
	s := stats.GetStats()
	if s.ProcessorErrors != 1 {
		t.Errorf("expected 1 processor error, got %d", s.ProcessorErrors)
	}
}

func TestStatsProcessor_NoRecordAsBatch(t *testing.T) {
	stats := batch.NewBasicStatsCollector()
	innerProc := &testProcessor{}

	statsProc := &processor.StatsProcessor{
		Processor:     innerProc,
		Stats:         stats,
		RecordAsBatch: false, // Don't record batch metrics
	}

	items := []*batch.Item{
		{ID: 1, Data: "one"},
		{ID: 2, Data: "two"},
	}

	ctx := context.Background()
	_, err := statsProc.Process(ctx, items)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Check stats - should have items but no batches
	s := stats.GetStats()
	if s.BatchesStarted != 0 {
		t.Errorf("expected 0 batches started, got %d", s.BatchesStarted)
	}
	if s.ItemsProcessed != 2 {
		t.Errorf("expected 2 items processed, got %d", s.ItemsProcessed)
	}
}

func TestStatsProcessor_NoStats(t *testing.T) {
	innerProc := &testProcessor{}
	statsProc := &processor.StatsProcessor{
		Processor: innerProc,
		Stats:     nil, // No stats collector
	}

	items := []*batch.Item{{ID: 1, Data: "test"}}
	ctx := context.Background()
	result, err := statsProc.Process(ctx, items)

	// Should work fine without stats
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 item, got %d", len(result))
	}
}

func TestStatsProcessor_NoProcessor(t *testing.T) {
	stats := batch.NewBasicStatsCollector()
	statsProc := &processor.StatsProcessor{
		Processor: nil,
		Stats:     stats,
	}

	items := []*batch.Item{{ID: 1, Data: "test"}}
	ctx := context.Background()
	result, err := statsProc.Process(ctx, items)

	// Should return items unchanged
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 item, got %d", len(result))
	}
}

func TestWrapWithStats(t *testing.T) {
	stats := batch.NewBasicStatsCollector()
	innerProc := &testProcessor{}

	wrapped := processor.WrapWithStats(innerProc, stats, true)

	if wrapped.Processor != innerProc {
		t.Error("inner processor not set correctly")
	}
	if wrapped.Stats != stats {
		t.Error("stats not set correctly")
	}
	if !wrapped.RecordAsBatch {
		t.Error("RecordAsBatch not set correctly")
	}
}
