package batch_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	. "github.com/MasterOfBinary/gobatch/batch"
)

// testSource emits predefined items with optional delay and final error.
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

// countProcessor counts processed items and can inject delays or errors.
type countProcessor struct {
	count        *uint32
	delay        time.Duration
	processorErr error
}

func (p *countProcessor) Process(ctx context.Context, items []*Item) ([]*Item, error) {
	if p.delay > 0 {
		time.Sleep(p.delay)
	}
	if p.count != nil {
		atomic.AddUint32(p.count, uint32(len(items)))
	}
	if p.processorErr != nil {
		return items, p.processorErr
	}
	return items, nil
}

// errorPerItemProcessor marks every n-th item as failed.
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

// transformProcessor modifies item data with transformFn.
type transformProcessor struct {
	transformFn func(interface{}) interface{}
}

func (p *transformProcessor) Process(ctx context.Context, items []*Item) ([]*Item, error) {
	select {
	case <-ctx.Done():
		return items, ctx.Err()
	default:
	}
	for _, item := range items {
		if item.Error != nil {
			continue
		}
		item.Data = p.transformFn(item.Data)
	}
	return items, nil
}

// filterProcessor only keeps items for which filterFn returns true.
type filterProcessor struct {
	filterFn func(interface{}) bool
}

func (p *filterProcessor) Process(ctx context.Context, items []*Item) ([]*Item, error) {
	var result []*Item
	for _, item := range items {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		if item.Error != nil {
			result = append(result, item)
			continue
		}
		if p.filterFn(item.Data) {
			result = append(result, item)
		}
	}
	return result, nil
}

// testProcessor calls processFn for processing items.
type testProcessor struct {
	processFn func(context.Context, []*Item) ([]*Item, error)
}

func (p *testProcessor) Process(ctx context.Context, items []*Item) ([]*Item, error) {
	if p.processFn != nil {
		return p.processFn(ctx, items)
	}
	return items, nil
}
