package batch_test

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/MasterOfBinary/gobatch/batch"
)

func TestLogLevel_String(t *testing.T) {
	tests := []struct {
		level    batch.LogLevel
		expected string
	}{
		{batch.LogLevelDebug, "DEBUG"},
		{batch.LogLevelInfo, "INFO"},
		{batch.LogLevelWarn, "WARN"},
		{batch.LogLevelError, "ERROR"},
		{batch.LogLevel(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.level.String(); got != tt.expected {
				t.Errorf("LogLevel.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestNoOpLogger(t *testing.T) {
	logger := &batch.NoOpLogger{}

	// These should not panic
	logger.Log(batch.LogLevelInfo, "test")
	logger.Debug("debug %d", 1)
	logger.Info("info %s", "test")
	logger.Warn("warn %v", true)
	logger.Error("error %f", 3.14)
}

func TestSimpleLogger(t *testing.T) {
	tests := []struct {
		name        string
		minLevel    batch.LogLevel
		logFunc     func(logger batch.Logger)
		contains    []string
		notContains []string
	}{
		{
			name:     "debug level allows all",
			minLevel: batch.LogLevelDebug,
			logFunc: func(logger batch.Logger) {
				logger.Debug("debug message")
				logger.Info("info message")
				logger.Warn("warn message")
				logger.Error("error message")
			},
			contains: []string{"[DEBUG]", "[INFO]", "[WARN]", "[ERROR]"},
		},
		{
			name:     "info level filters debug",
			minLevel: batch.LogLevelInfo,
			logFunc: func(logger batch.Logger) {
				logger.Debug("debug message")
				logger.Info("info message")
			},
			contains:    []string{"[INFO] info message"},
			notContains: []string{"[DEBUG]"},
		},
		{
			name:     "error level only shows errors",
			minLevel: batch.LogLevelError,
			logFunc: func(logger batch.Logger) {
				logger.Debug("debug")
				logger.Info("info")
				logger.Warn("warn")
				logger.Error("error message")
			},
			contains:    []string{"[ERROR] error message"},
			notContains: []string{"[DEBUG]", "[INFO]", "[WARN]"},
		},
		{
			name:     "formatting works",
			minLevel: batch.LogLevelInfo,
			logFunc: func(logger batch.Logger) {
				logger.Info("number: %d, string: %s", 42, "hello")
			},
			contains: []string{"[INFO] number: 42, string: hello"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture output
			var stdout, stderr bytes.Buffer
			logger := &batch.SimpleLogger{
				MinLevel:     tt.minLevel,
				StdoutLogger: log.New(&stdout, "", 0),
				StderrLogger: log.New(&stderr, "", 0),
			}

			tt.logFunc(logger)

			output := stdout.String() + stderr.String()

			for _, want := range tt.contains {
				if !strings.Contains(output, want) {
					t.Errorf("output missing expected string %q\nGot: %s", want, output)
				}
			}

			for _, notWant := range tt.notContains {
				if strings.Contains(output, notWant) {
					t.Errorf("output contains unexpected string %q\nGot: %s", notWant, output)
				}
			}
		})
	}
}

func TestSimpleLogger_OutputDestination(t *testing.T) {
	var stdout, stderr bytes.Buffer
	logger := &batch.SimpleLogger{
		MinLevel:     batch.LogLevelDebug,
		StdoutLogger: log.New(&stdout, "", 0),
		StderrLogger: log.New(&stderr, "", 0),
	}

	logger.Debug("debug to stdout")
	logger.Info("info to stdout")
	logger.Warn("warn to stderr")
	logger.Error("error to stderr")

	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	// Check stdout has debug and info
	if !strings.Contains(stdoutStr, "debug to stdout") {
		t.Error("stdout missing debug message")
	}
	if !strings.Contains(stdoutStr, "info to stdout") {
		t.Error("stdout missing info message")
	}

	// Check stderr has warn and error
	if !strings.Contains(stderrStr, "warn to stderr") {
		t.Error("stderr missing warn message")
	}
	if !strings.Contains(stderrStr, "error to stderr") {
		t.Error("stderr missing error message")
	}

	// Check no cross-contamination
	if strings.Contains(stdoutStr, "stderr") {
		t.Error("stdout contains stderr message")
	}
	if strings.Contains(stderrStr, "stdout") {
		t.Error("stderr contains stdout message")
	}
}
