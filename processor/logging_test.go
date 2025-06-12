package processor_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/processor"
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

type testProcessor struct {
	processFunc func(context.Context, []*batch.Item) ([]*batch.Item, error)
}

func (p *testProcessor) Process(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
	if p.processFunc != nil {
		return p.processFunc(ctx, items)
	}
	return items, nil
}

func TestLoggingProcessor_Success(t *testing.T) {
	logger := &captureLogger{}
	innerProc := &testProcessor{
		processFunc: func(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
			// Process successfully
			return items, nil
		},
	}

	logProc := &processor.LoggingProcessor{
		Processor: innerProc,
		Logger:    logger,
		Name:      "TestProcessor",
	}

	// Create test items
	items := []*batch.Item{
		{ID: 1, Data: "one"},
		{ID: 2, Data: "two"},
		{ID: 3, Data: "three"},
	}

	ctx := context.Background()
	result, err := logProc.Process(ctx, items)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(result) != len(items) {
		t.Errorf("expected %d items, got %d", len(items), len(result))
	}

	// Check logging
	messages := logger.getMessages()
	if len(messages) != 2 {
		t.Errorf("expected 2 log messages, got %d", len(messages))
		for i, msg := range messages {
			t.Logf("Message %d: %s", i, msg)
		}
	}

	// Should have start and complete messages
	foundStart := false
	foundComplete := false
	for _, msg := range messages {
		if strings.Contains(msg, "starting with 3 items") {
			foundStart = true
		}
		if strings.Contains(msg, "completed") && strings.Contains(msg, "3 successful") {
			foundComplete = true
		}
	}

	if !foundStart {
		t.Error("missing start log message")
		for _, msg := range messages {
			t.Logf("Message: %s", msg)
		}
	}
	if !foundComplete {
		t.Error("missing complete log message")
	}
}

func TestLoggingProcessor_Error(t *testing.T) {
	logger := &captureLogger{}
	testErr := errors.New("test error")
	innerProc := &testProcessor{
		processFunc: func(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
			return items, testErr
		},
	}

	logProc := &processor.LoggingProcessor{
		Processor: innerProc,
		Logger:    logger,
		Name:      "ErrorProcessor",
	}

	items := []*batch.Item{{ID: 1, Data: "test"}}
	ctx := context.Background()
	_, err := logProc.Process(ctx, items)

	if err != testErr {
		t.Errorf("expected error %v, got %v", testErr, err)
	}

	// Should have error log
	foundError := false
	messages := logger.getMessages()
	for _, msg := range messages {
		if strings.Contains(msg, "ERROR") && strings.Contains(msg, "failed") {
			foundError = true
			break
		}
	}

	if !foundError {
		t.Error("missing error log message")
	}
}

func TestLoggingProcessor_ItemErrors(t *testing.T) {
	logger := &captureLogger{}
	innerProc := &testProcessor{
		processFunc: func(ctx context.Context, items []*batch.Item) ([]*batch.Item, error) {
			// Mark some items as failed
			items[1].Error = errors.New("item error")
			items[3].Error = errors.New("another error")
			return items, nil
		},
	}

	logProc := &processor.LoggingProcessor{
		Processor: innerProc,
		Logger:    logger,
	}

	items := []*batch.Item{
		{ID: 1, Data: "one"},
		{ID: 2, Data: "two"},
		{ID: 3, Data: "three"},
		{ID: 4, Data: "four"},
	}

	ctx := context.Background()
	_, err := logProc.Process(ctx, items)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Check complete log mentions errors
	foundCompleteWithErrors := false
	messages := logger.getMessages()
	for _, msg := range messages {
		if strings.Contains(msg, "2 successful") && strings.Contains(msg, "2 errors") {
			foundCompleteWithErrors = true
			break
		}
	}

	if !foundCompleteWithErrors {
		t.Error("complete log should mention item errors")
		for _, msg := range messages {
			t.Logf("Message: %s", msg)
		}
	}
}

func TestLoggingProcessor_NoLogger(t *testing.T) {
	innerProc := &testProcessor{}
	logProc := &processor.LoggingProcessor{
		Processor: innerProc,
		Logger:    nil, // No logger
	}

	items := []*batch.Item{{ID: 1, Data: "test"}}
	ctx := context.Background()
	result, err := logProc.Process(ctx, items)

	// Should work fine without logging
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 item, got %d", len(result))
	}
}

func TestLoggingProcessor_NoProcessor(t *testing.T) {
	logger := &captureLogger{}
	logProc := &processor.LoggingProcessor{
		Processor: nil,
		Logger:    logger,
	}

	items := []*batch.Item{{ID: 1, Data: "test"}}
	ctx := context.Background()
	result, err := logProc.Process(ctx, items)

	// Should return items unchanged
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 item, got %d", len(result))
	}
}

func TestWrapWithLogging(t *testing.T) {
	logger := &captureLogger{}
	innerProc := &testProcessor{}

	wrapped := processor.WrapWithLogging(innerProc, logger, "Wrapped")

	if wrapped.Processor != innerProc {
		t.Error("inner processor not set correctly")
	}
	if wrapped.Logger != logger {
		t.Error("logger not set correctly")
	}
	if wrapped.Name != "Wrapped" {
		t.Error("name not set correctly")
	}
}
