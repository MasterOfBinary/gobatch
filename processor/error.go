package processor

import (
	"context"

	"github.com/MasterOfBinary/gobatch/batch"
)

// Error returns a Processor that returns an error while processing.
type Error struct {
	Err error
}

// Process discards all data sent to it after a certain amount of time.
func (p *Error) Process(ctx context.Context, ps *batch.PipelineStage) {
	ps.Errors <- p.Err
	ps.Close()
}
