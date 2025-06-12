package batch

import (
	"fmt"
	"log"
	"os"
)

// LogLevel represents the severity of a log message.
type LogLevel int

const (
	// LogLevelDebug is for detailed information, typically of interest only when diagnosing problems.
	LogLevelDebug LogLevel = iota
	// LogLevelInfo is for informational messages that highlight the progress of the application.
	LogLevelInfo
	// LogLevelWarn is for potentially harmful situations that might require attention.
	LogLevelWarn
	// LogLevelError is for error events that might still allow the application to continue running.
	LogLevelError
)

// String returns the string representation of the log level.
func (l LogLevel) String() string {
	switch l {
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARN"
	case LogLevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger defines the interface for logging within the batch processing pipeline.
// Implementations can route logs to various destinations (stdout, files, external services).
// The Logger is optional - if not provided, no logging occurs.
type Logger interface {
	// Log writes a log message at the specified level.
	// The message is formatted using fmt.Sprintf if args are provided.
	Log(level LogLevel, format string, args ...interface{})

	// Debug logs a debug-level message.
	Debug(format string, args ...interface{})

	// Info logs an info-level message.
	Info(format string, args ...interface{})

	// Warn logs a warning-level message.
	Warn(format string, args ...interface{})

	// Error logs an error-level message.
	Error(format string, args ...interface{})
}

// NoOpLogger is a logger that discards all log messages.
// It implements the Logger interface but performs no operations.
// This is the default logger when none is specified.
type NoOpLogger struct{}

// Log implements the Logger interface.
func (n *NoOpLogger) Log(level LogLevel, format string, args ...interface{}) {}

// Debug implements the Logger interface.
func (n *NoOpLogger) Debug(format string, args ...interface{}) {}

// Info implements the Logger interface.
func (n *NoOpLogger) Info(format string, args ...interface{}) {}

// Warn implements the Logger interface.
func (n *NoOpLogger) Warn(format string, args ...interface{}) {}

// Error implements the Logger interface.
func (n *NoOpLogger) Error(format string, args ...interface{}) {}

// SimpleLogger is a basic logger implementation that writes to stdout/stderr.
// Debug and Info messages go to stdout, Warn and Error messages go to stderr.
// Each log line includes a timestamp, log level, and the formatted message.
type SimpleLogger struct {
	// MinLevel is the minimum log level to output. Messages below this level are discarded.
	MinLevel LogLevel

	// StdoutLogger handles Debug and Info level messages
	StdoutLogger *log.Logger

	// StderrLogger handles Warn and Error level messages
	StderrLogger *log.Logger
}

// NewSimpleLogger creates a new SimpleLogger with the specified minimum log level.
// It uses the standard log package formatting with timestamps.
func NewSimpleLogger(minLevel LogLevel) *SimpleLogger {
	return &SimpleLogger{
		MinLevel:     minLevel,
		StdoutLogger: log.New(os.Stdout, "", log.LstdFlags),
		StderrLogger: log.New(os.Stderr, "", log.LstdFlags),
	}
}

// Log implements the Logger interface.
func (s *SimpleLogger) Log(level LogLevel, format string, args ...interface{}) {
	if level < s.MinLevel {
		return
	}

	msg := fmt.Sprintf(format, args...)
	prefix := fmt.Sprintf("[%s] ", level.String())

	switch level {
	case LogLevelDebug, LogLevelInfo:
		s.StdoutLogger.Printf("%s%s", prefix, msg)
	case LogLevelWarn, LogLevelError:
		s.StderrLogger.Printf("%s%s", prefix, msg)
	}
}

// Debug implements the Logger interface.
func (s *SimpleLogger) Debug(format string, args ...interface{}) {
	s.Log(LogLevelDebug, format, args...)
}

// Info implements the Logger interface.
func (s *SimpleLogger) Info(format string, args ...interface{}) {
	s.Log(LogLevelInfo, format, args...)
}

// Warn implements the Logger interface.
func (s *SimpleLogger) Warn(format string, args ...interface{}) {
	s.Log(LogLevelWarn, format, args...)
}

// Error implements the Logger interface.
func (s *SimpleLogger) Error(format string, args ...interface{}) {
	s.Log(LogLevelError, format, args...)
}
