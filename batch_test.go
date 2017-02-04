package gobatch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MasterOfBinary/gobatch/processor"
	"github.com/MasterOfBinary/gobatch/source"
)

type sourceFromSlice struct {
	slice    []interface{}
	duration time.Duration
}

func (s *sourceFromSlice) Read(ctx context.Context, items chan<- interface{}, errs chan<- error) {
	defer close(items)
	defer close(errs)

	for _, item := range s.slice {
		time.Sleep(s.duration)
		items <- item
	}
}

func TestMust(t *testing.T) {
	batch, _ := New(nil)

	if Must(batch, nil) != batch {
		t.Error("Must(batch, nil) != batch")
	}

	var panics bool
	func() {
		defer func() {
			if p := recover(); p != nil {
				panics = true
			}
		}()
		_ = Must(batch, errors.New("error"))
	}()

	if !panics {
		t.Error("Must(batch, err) doesn't panic")
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		config  *BatchConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: false,
		},
		{
			name:    "empty config",
			config:  &BatchConfig{},
			wantErr: false,
		},
		{
			name: "good config",
			config: &BatchConfig{
				MinItems:        5,
				MaxItems:        10,
				MinTime:         time.Second,
				MaxTime:         2 * time.Second,
				ReadConcurrency: 5,
			},
			wantErr: false,
		},
		{
			name: "min time only",
			config: &BatchConfig{
				MinTime: time.Second,
			},
			wantErr: false,
		},
		{
			name: "min items only",
			config: &BatchConfig{
				MinItems: 5,
			},
			wantErr: false,
		},
		{
			name: "bad items",
			config: &BatchConfig{
				MinItems: 10,
				MaxItems: 5,
			},
			wantErr: true,
		},
		{
			name: "bad times",
			config: &BatchConfig{
				MinTime: 2 * time.Second,
				MaxTime: time.Second,
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			batch, err := New(test.config)
			if test.wantErr && err == nil {
				t.Error("New(config) returns nil error, want not nil")
			} else if !test.wantErr {
				if err != nil {
					t.Errorf("New(config) returns error %v, want nil", err)
				}
				if batch == nil {
					t.Error("New(config) returns nil batch, want not nil")
				}
			}
		})
	}
}

func TestBatch_Go(t *testing.T) {
	t.Run("concurrent calls", func(t *testing.T) {
		t.Parallel()

		// Concurrent calls to Go should panic
		batch := &Batch{}
		s := source.Nil(time.Second)
		p := processor.Nil(0)

		IgnoreErrors(batch.Go(context.Background(), s, p))

		// Next call should panic
		var panics bool
		func() {
			defer func() {
				if p := recover(); p != nil {
					panics = true
				}
			}()
			IgnoreErrors(batch.Go(context.Background(), s, p))
		}()

		if !panics {
			t.Error("Concurrent calls to batch.Go don't panic")
		}
	})

	t.Run("source error", func(t *testing.T) {
		t.Parallel()

		errSrc := errors.New("source")
		batch := &Batch{}
		s := source.Error(errSrc)
		p := processor.Nil(0)

		errs := batch.Go(context.Background(), s, p)

		var found bool
		for err := range errs {
			if src, ok := err.(*SourceError); ok {
				if src.Original() == errSrc {
					found = true
				} else {
					t.Error("Found source error %v, want %v", src.Original(), errSrc)
				}
			} else {
				t.Error("Found an unexpected error")
			}
		}

		if !found {
			t.Error("Did not find source error")
		}
	})

	t.Run("processor error", func(t *testing.T) {
		t.Parallel()

		errProc := errors.New("processor")
		batch := &Batch{}
		s := &sourceFromSlice{
			slice: []interface{}{1},
		}
		p := processor.Error(errProc)

		errs := batch.Go(context.Background(), s, p)

		var found bool
		for err := range errs {
			if proc, ok := err.(*ProcessorError); ok {
				if proc.Original() == errProc {
					found = true
				} else {
					t.Error("Found processor error %v, want %v", proc.Original(), errProc)
				}
			} else {
				t.Error("Found an unexpected error")
			}
		}

		if !found {
			t.Error("Did not find processor error")
		}
	})
}

func TestBatch_Done(t *testing.T) {
	t.Run("basic test", func(t *testing.T) {
		t.Parallel()

		batch := &Batch{}
		s := source.Nil(0)
		p := processor.Nil(0)

		IgnoreErrors(batch.Go(context.Background(), s, p))

		select {
		case <-batch.Done():
			break
		case <-time.After(time.Second):
			t.Error("Done channel never closed")
		}
	})

	t.Run("with source sleep", func(t *testing.T) {
		t.Parallel()

		batch := &Batch{}
		s := &sourceFromSlice{
			slice:    []interface{}{1},
			duration: 100 * time.Millisecond,
		}
		p := processor.Nil(10 * time.Millisecond)

		timer := time.After(100 * time.Millisecond)
		IgnoreErrors(batch.Go(context.Background(), s, p))

		select {
		case <-batch.Done():
			t.Error("Done channel closed before source")
		case <-timer:
			break
		case <-time.After(time.Second):
			t.Error("Done channel never closed")
		}
	})

	t.Run("with processor sleep", func(t *testing.T) {
		t.Parallel()

		batch := &Batch{}
		s := &sourceFromSlice{
			slice:    []interface{}{1},
			duration: 10 * time.Millisecond,
		}
		p := processor.Nil(100 * time.Millisecond)

		timer := time.After(100 * time.Millisecond)
		IgnoreErrors(batch.Go(context.Background(), s, p))

		select {
		case <-batch.Done():
			t.Error("Done channel closed before processor")
		case <-timer:
			break
		case <-time.After(time.Second):
			t.Error("Done channel never closed")
		}
	})
}
