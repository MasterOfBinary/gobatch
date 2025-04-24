package batch_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/MasterOfBinary/gobatch/batch"
)

type testSource struct {
	Items   []interface{}
	Delay   time.Duration
	WithErr error
}

func (s *testSource) Read(ctx context.Context) (<-chan interface{}, <-chan error) {
	out := make(chan interface{})
	errs := make(chan error, 1)
	go func() {
		defer close(out)
		defer close(errs)
		for _, item := range s.Items {
			if s.Delay > 0 {
				time.Sleep(s.Delay)
			}
			select {
			case <-ctx.Done():
				return
			case out <- item:
			}
		}
		if s.WithErr != nil {
			errs <- s.WithErr
		}
	}()
	return out, errs
}

type countProcessor struct {
	count *uint32
}

func (p *countProcessor) Process(ctx context.Context, items []*Item) ([]*Item, error) {
	atomic.AddUint32(p.count, uint32(len(items)))
	return items, nil
}

type errorPerItemProcessor struct {
	FailEvery int
}

func (p *errorPerItemProcessor) Process(ctx context.Context, items []*Item) ([]*Item, error) {
	for i, item := range items {
		if p.FailEvery > 0 && (i%p.FailEvery) == 0 {
			item.Error = fmt.Errorf("fail item %d", item.ID)
		}
	}
	return items, nil
}

func TestBatch_ProcessorChainingAndErrorTracking(t *testing.T) {
	t.Run("processor chaining with individual errors", func(t *testing.T) {
		var count uint32
		batch := New(NewConstantConfig(&ConfigValues{
			MinItems: 5,
		}))
		src := &testSource{Items: []interface{}{1, 2, 3, 4, 5, 6, 7, 8, 9}}
		errProc := &errorPerItemProcessor{FailEvery: 3}
		countProc := &countProcessor{count: &count}

		errs := batch.Go(context.Background(), src, errProc, countProc)

		received := 0
		for err := range errs {
			var processorError *ProcessorError
			if !errors.As(err, &processorError) {
				t.Errorf("unexpected error type: %v", err)
			}
			received++
		}
		if received != 2 {
			t.Errorf("expected 2 item errors, got %d", received)
		}

		if atomic.LoadUint32(&count) != 5 {
			t.Errorf("expected 5 items processed, got %d", count)
		}

		<-batch.Done()
	})

	t.Run("source error forwarding", func(t *testing.T) {
		srcErr := errors.New("source failed")
		batch := New(NewConstantConfig(&ConfigValues{}))
		src := &testSource{Items: []interface{}{1, 2}, WithErr: srcErr}
		countProc := &countProcessor{count: new(uint32)}

		errs := batch.Go(context.Background(), src, countProc)
		<-batch.Done()

		var found bool
		for err := range errs {
			var sourceError *SourceError
			if errors.As(err, &sourceError) {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected to find source error")
		}
	})

	t.Run("batching scenarios", func(t *testing.T) {
		configs := []struct {
			name     string
			config   *ConfigValues
			duration time.Duration
			size     int
			expected int
		}{
			{"min items", &ConfigValues{MinItems: 5}, 0, 10, 10},
			{"max items", &ConfigValues{MaxItems: 3}, 0, 9, 9},
			{"min time", &ConfigValues{MinTime: 300 * time.Millisecond}, 100 * time.Millisecond, 5, 2},
			{"max time", &ConfigValues{MaxTime: 200 * time.Millisecond}, 150 * time.Millisecond, 3, 3},
			{"eof fallback", &ConfigValues{MinItems: 10}, 0, 5, 0},
			{"min items and max time", &ConfigValues{MinItems: 5, MaxTime: 200 * time.Millisecond}, 100 * time.Millisecond, 3, 1},
			{"max items and min time", &ConfigValues{MaxItems: 3, MinTime: 500 * time.Millisecond}, 100 * time.Millisecond, 5, 3},
			{"min and max time interaction", &ConfigValues{MinTime: 500 * time.Millisecond, MaxTime: 300 * time.Millisecond}, 100 * time.Millisecond, 5, 2},
			{"min and max items interaction", &ConfigValues{MinItems: 5, MaxItems: 3}, 0, 10, 9},
			{"all thresholds", &ConfigValues{MinItems: 3, MaxItems: 5, MinTime: 200 * time.Millisecond, MaxTime: 400 * time.Millisecond}, 100 * time.Millisecond, 7, 6},
		}

		for _, tt := range configs {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				var count uint32
				items := make([]interface{}, tt.size)
				for i := 0; i < tt.size; i++ {
					items[i] = rand.Int()
				}

				batch := New(NewConstantConfig(tt.config))
				src := &testSource{Items: items, Delay: tt.duration}
				proc := &countProcessor{count: &count}

				_ = batch.Go(context.Background(), src, proc)
				<-batch.Done()

				got := int(atomic.LoadUint32(&count))
				if got != tt.expected {
					t.Errorf("got %d items processed, expected %d", got, tt.expected)
				}
			})
		}
	})
}
