package batch_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	. "github.com/MasterOfBinary/gobatch/batch"
)

func TestBatch_ErrorHandling(t *testing.T) {
	t.Run("processor with error", func(t *testing.T) {
		batch := New(NewConstantConfig(&ConfigValues{}))
		src := &TestSource{Items: []interface{}{1, 2, 3, 4, 5}}

		procErr := errors.New("processor error")
		proc := &CountProcessor{
			Count:        new(uint32),
			ProcessorErr: procErr,
		}

		errs := batch.Go(context.Background(), src, proc)

		var foundErr bool
		for err := range errs {
			if err != nil && errors.Unwrap(err) == procErr {
				foundErr = true
				break
			}
		}

		<-batch.Done()

		if !foundErr {
			t.Error("expected to find processor error")
		}
	})

	t.Run("source with error", func(t *testing.T) {
		batch := New(NewConstantConfig(&ConfigValues{}))

		srcErr := errors.New("source error")
		src := &TestSource{
			Items:   []interface{}{1, 2, 3},
			WithErr: srcErr,
		}

		proc := &CountProcessor{Count: new(uint32)}

		errs := batch.Go(context.Background(), src, proc)

		var foundErr bool
		for err := range errs {
			if err != nil && errors.Unwrap(err) == srcErr {
				foundErr = true
				break
			}
		}

		<-batch.Done()

		if !foundErr {
			t.Error("expected to find source error")
		}
	})

	t.Run("nil source handling", func(t *testing.T) {
		batch := New(NewConstantConfig(&ConfigValues{}))

		// Pass nil source
		errs := batch.Go(context.Background(), nil)

		var foundErr bool
		var errMsg string
		for err := range errs {
			if err != nil {
				foundErr = true
				errMsg = err.Error()
				break
			}
		}

		<-batch.Done()

		if !foundErr {
			t.Error("expected error with nil source")
		}

		if !strings.Contains(errMsg, "source cannot be nil") {
			t.Errorf("expected 'source cannot be nil' error, got: %s", errMsg)
		}
	})

	t.Run("nil processor filtering", func(t *testing.T) {
		batch := New(NewConstantConfig(&ConfigValues{}))
		src := &TestSource{Items: []interface{}{1, 2, 3}}

		// Include a nil processor among valid ones
		var count uint32
		validProc := &CountProcessor{Count: &count}

		// Pass a mix of nil and valid processors
		errs := batch.Go(context.Background(), src, nil, validProc, nil)

		// Count errors instead of collecting them
		errorCount := 0
		for range errs {
			errorCount++
		}

		<-batch.Done()

		// Valid processor should still run
		if atomic.LoadUint32(&count) != 3 {
			t.Errorf("expected 3 items processed, got %d", count)
		}

		// The batch should process without errors
		if errorCount > 0 {
			t.Errorf("expected no errors with empty processor slice, got %d errors", errorCount)
		}
	})
}

func TestBatch_NilChannelHandling(t *testing.T) {
	t.Run("source returning nil output channel", func(t *testing.T) {
		batch := New(NewConstantConfig(&ConfigValues{}))

		// Create a source that returns a nil output channel
		nilChannelSource := &nilOutputChannelSource{}

		errs := batch.Go(context.Background(), nilChannelSource)

		var foundErr bool
		var errMsg string
		for err := range errs {
			if err != nil {
				foundErr = true
				errMsg = err.Error()
				break
			}
		}

		<-batch.Done()

		if !foundErr {
			t.Error("expected error with nil output channel")
		}

		if !strings.Contains(errMsg, "nil channel") {
			t.Errorf("expected error about nil channels, got: %s", errMsg)
		}
	})

	t.Run("source returning nil error channel", func(t *testing.T) {
		batch := New(NewConstantConfig(&ConfigValues{}))

		// Create a source that returns a nil error channel
		nilChannelSource := &nilErrorChannelSource{}

		errs := batch.Go(context.Background(), nilChannelSource)

		var foundErr bool
		var errMsg string
		for err := range errs {
			if err != nil {
				foundErr = true
				errMsg = err.Error()
				break
			}
		}

		<-batch.Done()

		if !foundErr {
			t.Error("expected error with nil error channel")
		}

		if !strings.Contains(errMsg, "nil channel") {
			t.Errorf("expected error about nil channels, got: %s", errMsg)
		}
	})
}

// Source that returns a nil output channel and a valid error channel
type nilOutputChannelSource struct{}

func (s *nilOutputChannelSource) Read(ctx context.Context) (<-chan interface{}, <-chan error) {
	errs := make(chan error)
	close(errs)
	return nil, errs
}

// Source that returns a valid output channel and a nil error channel
type nilErrorChannelSource struct{}

func (s *nilErrorChannelSource) Read(ctx context.Context) (<-chan interface{}, <-chan error) {
	out := make(chan interface{})
	close(out)
	return out, nil
}
