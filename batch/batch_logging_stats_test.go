package batch_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"testing"

	"github.com/MasterOfBinary/gobatch/batch"
)

type captureLogger struct {
	mu       sync.Mutex
	messages []string
}

func (c *captureLogger) Log(level batch.LogLevel, format string, args ...interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	msg := fmt.Sprintf(format, args...)
	c.messages = append(c.messages, level.String()+" "+msg)
}
func (c *captureLogger) Debug(format string, args ...interface{}) {
	c.Log(batch.LogLevelDebug, format, args...)
}
func (c *captureLogger) Info(format string, args ...interface{}) {
	c.Log(batch.LogLevelInfo, format, args...)
}
func (c *captureLogger) Warn(format string, args ...interface{}) {
	c.Log(batch.LogLevelWarn, format, args...)
}
func (c *captureLogger) Error(format string, args ...interface{}) {
	c.Log(batch.LogLevelError, format, args...)
}

func (c *captureLogger) getMessages() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]string, len(c.messages))
	copy(result, c.messages)
	return result
}

func TestBatch_WithLogger(t *testing.T) {
	logger := &captureLogger{}
	b := batch.New(nil).WithLogger(logger)

	// Create simple source and processor
	src := &testSource{Items: []interface{}{1, 2, 3}}
	proc := &countProcessor{}

	ctx := context.Background()
	batch.IgnoreErrors(b.Go(ctx, src, proc))
	<-b.Done()

	// Check that logging occurred
	found := false
	messages := logger.getMessages()
	for _, msg := range messages {
		if strings.Contains(msg, "Starting batch processing") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected log message not found")
	}
}

func TestBatch_WithStats(t *testing.T) {
	stats := batch.NewBasicStatsCollector()

	// Use a config that batches items together
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 3,
		MaxItems: 5,
	})
	b := batch.New(config).WithStats(stats)

	// Create source with 10 items
	items := make([]interface{}, 10)
	for i := range items {
		items[i] = i
	}
	src := &testSource{Items: items}

	// Processor that fails some items
	var itemCount int
	var mu sync.Mutex
	proc := &testProcessor{
		processFn: func(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
			mu.Lock()
			defer mu.Unlock()
			for _, item := range items {
				itemCount++
				// Fail every 3rd item overall
				if itemCount%3 == 0 {
					item.Error = errors.New("test error")
				}
			}
			return items, nil
		},
	}

	ctx := context.Background()
	batch.IgnoreErrors(b.Go(ctx, src, proc))
	<-b.Done()

	// Verify stats
	s := stats.GetStats()
	if s.ItemsProcessed == 0 {
		t.Error("No items were recorded as processed")
	}
	if s.ItemErrors == 0 {
		t.Error("No item errors were recorded")
	}
	if s.BatchesCompleted == 0 {
		t.Error("No batches were recorded as completed")
	}

	// Verify counts make sense
	totalItems := s.ItemsProcessed + s.ItemErrors
	if totalItems != 10 {
		t.Errorf("Expected 10 total items, got %d (processed: %d, errors: %d)",
			totalItems, s.ItemsProcessed, s.ItemErrors)
	}
}

func TestBatch_LoggerAndStatsPanic(t *testing.T) {
	b := batch.New(nil)

	// Start processing
	ctx := context.Background()
	batch.IgnoreErrors(b.Go(ctx, &testSource{Items: []interface{}{1}}, &countProcessor{}))

	// These should panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("WithLogger did not panic after Go()")
		}
	}()
	b.WithLogger(&captureLogger{})
}

func TestBatch_WithLoggerAndStats_Integration(t *testing.T) {
	// Capture logger output
	var stdoutBuf, stderrBuf bytes.Buffer
	logger := &batch.SimpleLogger{
		MinLevel:     batch.LogLevelInfo,
		StdoutLogger: log.New(&stdoutBuf, "", 0),
		StderrLogger: log.New(&stderrBuf, "", 0),
	}

	// Create stats collector
	stats := batch.NewBasicStatsCollector()

	// Configure batch
	config := batch.NewConstantConfig(&batch.ConfigValues{
		MinItems: 2,
		MaxItems: 5,
	})

	b := batch.New(config).
		WithLogger(logger).
		WithStats(stats)

	// Create source with some items and an error
	src := &testSource{
		Items:   []interface{}{1, 2, 3, 4, 5},
		WithErr: errors.New("source error"),
	}

	// Processor that processes items
	var processedCount int
	var procMu sync.Mutex
	proc := &testProcessor{
		processFn: func(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
			procMu.Lock()
			processedCount += len(items)
			procMu.Unlock()
			// Mark one item as error
			if len(items) > 0 {
				items[0].Error = errors.New("item error")
			}
			return items, nil
		},
	}

	// Run batch
	ctx := context.Background()
	var errorCount int
	errs := b.Go(ctx, src, proc)
	for range errs {
		errorCount++
	}
	<-b.Done()

	// Verify logging output
	output := stdoutBuf.String() + stderrBuf.String()
	if !strings.Contains(output, "Starting batch processing") {
		t.Error("Missing start log")
	}
	if !strings.Contains(output, "Source reading complete") {
		t.Error("Missing source complete log")
	}
	if !strings.Contains(output, "Batch processing complete") {
		t.Error("Missing processing complete log")
	}
	if !strings.Contains(output, "Source error") {
		t.Error("Missing source error log")
	}

	// Verify stats
	s := stats.GetStats()
	if s.ItemsProcessed == 0 {
		t.Error("No items processed in stats")
	}
	if s.ItemErrors == 0 {
		t.Error("No item errors in stats")
	}
	if s.SourceErrors != 1 {
		t.Errorf("Expected 1 source error, got %d", s.SourceErrors)
	}
	if s.BatchesCompleted == 0 {
		t.Error("No batches completed")
	}

	// Verify processing occurred
	procMu.Lock()
	finalProcessedCount := processedCount
	procMu.Unlock()
	if finalProcessedCount != 5 {
		t.Errorf("Expected 5 items processed, got %d", finalProcessedCount)
	}
}

func TestBatch_DefaultLoggingAndStats(t *testing.T) {
	// Create batch without logger or stats
	b := batch.New(nil)

	// This should work fine with no-op implementations
	src := &testSource{Items: []interface{}{1, 2, 3}}
	proc := &countProcessor{}

	ctx := context.Background()
	batch.IgnoreErrors(b.Go(ctx, src, proc))
	<-b.Done()

	// Test passes if no panic occurred
}
