package processor

import (
	"context"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
)

// Nil is a Processor that discards all data after a specified duration.
// It can be used as a mock Processor.
type Nil[I, O any] struct {
	Duration time.Duration
}

// Process discards all data sent to it after a certain amount of time.
func (p *Nil[I, O]) Process(ctx context.Context, ps *batch.PipelineStage[I, O]) {
	time.Sleep(p.Duration)
	ps.Close()
}
