package batch

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

// TestSource emits predefined items with optional delay and final error.
type TestSource struct {
	Items   []interface{}
	Delay   time.Duration
	WithErr error
}

func (s *TestSource) Read(ctx context.Context) (<-chan interface{}, <-chan error) {
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

// CountProcessor counts processed items and can inject delays or errors.
type CountProcessor struct {
	Count        *uint32
	Delay        time.Duration
	ProcessorErr error
}

func (p *CountProcessor) Process(ctx context.Context, items []*Item) ([]*Item, error) {
	if p.Delay > 0 {
		time.Sleep(p.Delay)
	}
	if p.Count != nil {
		atomic.AddUint32(p.Count, uint32(len(items)))
	}
	if p.ProcessorErr != nil {
		return items, p.ProcessorErr
	}
	return items, nil
}

// ErrorPerItemProcessor marks every n-th item as failed.
type ErrorPerItemProcessor struct {
	FailEvery int
}

func (p *ErrorPerItemProcessor) Process(ctx context.Context, items []*Item) ([]*Item, error) {
	for i, item := range items {
		if p.FailEvery > 0 && (i%p.FailEvery) == 0 {
			item.Error = fmt.Errorf("fail item %d", item.ID)
		}
	}
	return items, nil
}

// TransformProcessor modifies item data with transformFn.
type TransformProcessor struct {
	TransformFn func(interface{}) interface{}
}

func (p *TransformProcessor) Process(ctx context.Context, items []*Item) ([]*Item, error) {
	select {
	case <-ctx.Done():
		return items, ctx.Err()
	default:
	}
	for _, item := range items {
		if item.Error != nil {
			continue
		}
		item.Data = p.TransformFn(item.Data)
	}
	return items, nil
}

// FilterProcessor only keeps items for which filterFn returns true.
type FilterProcessor struct {
	FilterFn func(interface{}) bool
}

func (p *FilterProcessor) Process(ctx context.Context, items []*Item) ([]*Item, error) {
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
		if p.FilterFn(item.Data) {
			result = append(result, item)
		}
	}
	return result, nil
}

// TestProcessor calls processFn for processing items.
type TestProcessor struct {
	ProcessFn func(context.Context, []*Item) ([]*Item, error)
}

func (p *TestProcessor) Process(ctx context.Context, items []*Item) ([]*Item, error) {
	if p.ProcessFn != nil {
		return p.ProcessFn(ctx, items)
	}
	return items, nil
}
