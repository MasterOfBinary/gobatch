package processor

import (
	"context"

	"github.com/MasterOfBinary/gobatch/batch"
)

// Error is a Processor that marks all incoming items with the given error.
type Error struct {
	Err error
}

func (p *Error) Process(_ context.Context, items []*batch.Item) []*batch.Item {
	for _, item := range items {
		item.Error = p.Err
	}
	return items
}
