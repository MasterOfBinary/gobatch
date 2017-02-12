package processor

import (
	"context"
	"time"

	"github.com/MasterOfBinary/gobatch/item"
)

type nilProcessor struct {
	duration time.Duration
}

// Nil returns a Processor that discards all data after a specified duration.
// It can be used as a mock Processor.
func Nil(duration time.Duration) Processor {
	return &nilProcessor{
		duration: duration,
	}
}

// Process discards all data sent to it after a certain amount of time.
func (p *nilProcessor) Process(ctx context.Context, items []item.Item, errs chan<- error) {
	time.Sleep(p.duration)
	close(errs)
}
