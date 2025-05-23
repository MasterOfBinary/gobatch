package batch

import (
	"context"
)

// ProcessorFunc is a function type that implements the Processor interface
type ProcessorFunc func(ctx context.Context, items []*Item) ([]*Item, error)

func (f ProcessorFunc) Process(ctx context.Context, items []*Item) ([]*Item, error) {
	return f(ctx, items)
}

// SourceFunc is a function type that implements the Source interface
type SourceFunc func(ctx context.Context) (<-chan interface{}, <-chan error)

func (f SourceFunc) Read(ctx context.Context) (<-chan interface{}, <-chan error) {
	return f(ctx)
}