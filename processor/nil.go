package processor

import (
	"context"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
)

// Nil is a Processor that sleeps for the configured duration and does nothing else.
type Nil struct {
	Duration time.Duration
}

func (p *Nil) Process(ctx context.Context, items []*batch.Item) []*batch.Item {
	if p.Duration > 0 {
		select {
		case <-time.After(p.Duration):
		case <-ctx.Done():
			for _, item := range items {
				item.Error = ctx.Err()
			}
		}
	}
	return items
}
