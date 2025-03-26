package batch_test

import (
	"context"
	"errors"
	"math/rand"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/MasterOfBinary/gobatch/batch"
	"github.com/MasterOfBinary/gobatch/processor"
	"github.com/MasterOfBinary/gobatch/source"
)

type sourceFromSlice struct {
	slice    []int
	duration time.Duration
}

func (s *sourceFromSlice) Read(ctx context.Context, ps *PipelineStage[int, int]) {
	defer ps.Close()

	for _, item := range s.slice {
		time.Sleep(s.duration)
		ps.Output <- NextItem[int, int](ps, item)
	}
}

type processorCounter struct {
	totalCount uint32
	num        uint32
}

func (p *processorCounter) Process(ctx context.Context, ps *PipelineStage[int, int]) {
	defer ps.Close()

	count := 0
	for range ps.Input {
		count++
	}

	atomic.AddUint32(&p.totalCount, uint32(count))
	atomic.AddUint32(&p.num, 1)
}

func (p *processorCounter) average() int {
	return int(float64(p.totalCount)/float64(p.num) + 0.5)
}

func assertNoErrors(t *testing.T, errs <-chan error) {
	if errs != nil {
		go func() {
			for err := range errs {
				t.Errorf("Unexpected error %v returned from batch.Go", err)
			}
		}()
	}
}

func TestBatch_Go(t *testing.T) {
	t.Run("basic test", func(t *testing.T) {
		t.Parallel()

		batch := &Batch[int, any]{}
		s := &sourceFromSlice{
			slice: []int{1, 2, 3, 4, 5},
		}
		p := &processor.Nil[int, any]{}

		errs := batch.Go(context.Background(), s, p)

		select {
		case err, ok := <-errs:
			if !ok {
				break
			} else {
				t.Errorf("Unexpected error %v returned from batch.Go", err.Error())
			}
		case <-time.After(time.Second):
			t.Error("err channel never closed")
		}
	})

	t.Run("concurrent calls", func(t *testing.T) {
		t.Parallel()

		// Concurrent calls to Go should panic
		batch := &Batch[int, any]{}
		s := &source.Nil[int]{
			Duration: time.Second,
		}
		p := &processor.Nil[int, any]{
			Duration: 0,
		}

		assertNoErrors(t, batch.Go(context.Background(), s, p))

		// Next call should panic
		var panics bool
		func() {
			defer func() {
				if p := recover(); p != nil {
					panics = true
				}
			}()
			assertNoErrors(t, batch.Go(context.Background(), s, p))
		}()

		if !panics {
			t.Error("Concurrent calls to batch.Go don't panic")
		}
	})

	t.Run("source error", func(t *testing.T) {
		t.Parallel()

		errSrc := errors.New("source")
		batch := &Batch[int, any]{}
		s := &source.Error[int]{
			Err: errSrc,
		}
		p := &processor.Nil[int, any]{}

		errs := batch.Go(context.Background(), s, p)

		var found bool
		for err := range errs {
			if src, ok := err.(*SourceError); ok {
				if src.Original() == errSrc {
					found = true
				} else {
					t.Errorf("Found source error %v, want %v", src.Original(), errSrc)
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
		batch := &Batch[int, any]{}
		s := &sourceFromSlice{
			slice: []int{1},
		}
		p := &processor.Error[int, any]{
			Err: errProc,
		}

		errs := batch.Go(context.Background(), s, p)

		var found bool
		for err := range errs {
			if proc, ok := err.(*ProcessorError); ok {
				if proc.Original() == errProc {
					found = true
				} else {
					t.Errorf("Found processor error %v, want %v", proc.Original(), errProc)
				}
			} else {
				t.Error("Found an unexpected error")
			}
		}

		if !found {
			t.Error("Did not find processor error")
		}
	})

	t.Run("scenarios", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name               string
			config             *ConfigValues
			inputSize          int
			inputDuration      time.Duration
			wantProcessingSize int
		}{
			{
				name:               "default",
				config:             nil,
				inputSize:          100,
				wantProcessingSize: 1,
			},
			{
				name: "min items",
				config: &ConfigValues{
					MinItems: 5,
				},
				inputSize:          100,
				wantProcessingSize: 5,
			},
			{
				name: "min time",
				config: &ConfigValues{
					MinTime: 250 * time.Millisecond,
				},
				inputSize:          6,
				inputDuration:      100 * time.Millisecond,
				wantProcessingSize: 2,
			},
			{
				name: "max items",
				config: &ConfigValues{
					MaxItems: 5,
				},
				inputSize:          100,
				wantProcessingSize: 1,
			},
			{
				name: "max time",
				config: &ConfigValues{
					MaxTime: 200 * time.Millisecond,
				},
				inputSize:          3,
				inputDuration:      150 * time.Millisecond,
				wantProcessingSize: 1,
			},
			{
				name: "min time and min items",
				config: &ConfigValues{
					MinTime:  450 * time.Millisecond,
					MinItems: 2,
				},
				inputSize:          8,
				inputDuration:      100 * time.Millisecond,
				wantProcessingSize: 4, // MinTime > MinItems
			},
			{
				name: "min time and max items",
				config: &ConfigValues{
					MinTime:  450 * time.Millisecond,
					MaxItems: 2,
				},
				inputSize:          8,
				inputDuration:      100 * time.Millisecond,
				wantProcessingSize: 2, // MaxItems > MinTime
			},
			{
				name: "min time and max time",
				config: &ConfigValues{
					MinTime: 450 * time.Millisecond,
					MaxTime: 1 * time.Second,
				},
				inputSize:          8,
				inputDuration:      100 * time.Millisecond,
				wantProcessingSize: 4, // MinTime > MaxTime
			},
			{
				name: "min items and max time",
				config: &ConfigValues{
					MinItems: 10,
					MaxTime:  200 * time.Millisecond,
				},
				inputSize:          3,
				inputDuration:      150 * time.Millisecond,
				wantProcessingSize: 1, // MaxTime > MinItems
			},
			{
				name: "min items and max items",
				config: &ConfigValues{
					MinItems: 10,
					MaxItems: 20,
				},
				inputSize:          20,
				inputDuration:      50 * time.Millisecond,
				wantProcessingSize: 10, // MaxItems > MinItems
			},
			{
				name: "min items and eof",
				config: &ConfigValues{
					MinItems: 10,
				},
				inputSize:          5,
				wantProcessingSize: 5, // EOF > MinItems
			},
			{
				name: "min time and eof",
				config: &ConfigValues{
					MinTime: 100 * time.Millisecond,
				},
				inputSize:          5,
				wantProcessingSize: 5, // EOF > MinTime
			},
		}

		for _, test := range tests {
			test := test
			t.Run(test.name, func(t *testing.T) {
				t.Parallel()

				inputSlice := make([]int, test.inputSize)
				for i := 0; i < len(inputSlice); i++ {
					inputSlice[i] = rand.Int()
				}

				batch := New[int, int](NewConstantConfig(test.config))
				s := &sourceFromSlice{
					slice:    inputSlice,
					duration: test.inputDuration,
				}
				p := &processorCounter{}

				assertNoErrors(t, batch.Go(context.Background(), s, p))

				<-batch.Done()

				got := p.average()
				if got != test.wantProcessingSize {
					t.Errorf("Average processing size = %v, want %v", got, test.wantProcessingSize)
				}
			})
		}
	})
}

func TestBatch_Done(t *testing.T) {
	t.Run("basic test", func(t *testing.T) {
		t.Parallel()

		batch := &Batch[int, any]{}
		s := &source.Nil[int]{
			Duration: 0,
		}
		p := &processor.Nil[int, any]{
			Duration: 0,
		}

		assertNoErrors(t, batch.Go(context.Background(), s, p))

		select {
		case <-batch.Done():
			break
		case <-time.After(time.Second):
			t.Error("Done channel never closed")
		}
	})

	t.Run("with source sleep", func(t *testing.T) {
		t.Parallel()

		batch := &Batch[int, any]{}
		s := &sourceFromSlice{
			slice:    []int{1},
			duration: 100 * time.Millisecond,
		}
		p := &processor.Nil[int, any]{
			Duration: 10 * time.Millisecond,
		}

		timer := time.After(100 * time.Millisecond)
		assertNoErrors(t, batch.Go(context.Background(), s, p))

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

		batch := &Batch[int, any]{}
		s := &sourceFromSlice{
			slice:    []int{1},
			duration: 10 * time.Millisecond,
		}
		p := &processor.Nil[int, any]{
			Duration: 100 * time.Millisecond,
		}

		timer := time.After(100 * time.Millisecond)
		assertNoErrors(t, batch.Go(context.Background(), s, p))

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
