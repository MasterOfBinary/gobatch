package processor

import (
	"context"

	"github.com/MasterOfBinary/gobatch/batch"
)

// Error returns a Processor that returns an error while processing.
type Error[I, O any] struct {
	Err error
}

// Process discards all data sent to it after a certain amount of time.
func (p *Error[I, O]) Process(ctx context.Context, ps *batch.PipelineStage[I, O]) {
	ps.Errors <- p.Err
	ps.Close()
}
