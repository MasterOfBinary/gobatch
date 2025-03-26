package source

import (
	"context"

	"github.com/MasterOfBinary/gobatch/batch"
)

// Channel is a Source that reads from input until it is closed.
// The input channel can be buffered or unbuffered.
type Channel[I any] struct {
	Input <-chan I
}

// Read reads from items until the input channel is closed.
func (s *Channel[I]) Read(_ context.Context, ps *batch.PipelineStage[I, I]) {
	defer ps.Close()

	out := ps.Output
	for item := range s.Input {
		out <- batch.NextItem(ps, item)
	}
}
