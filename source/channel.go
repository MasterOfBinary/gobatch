package source

import (
	"context"

	"github.com/MasterOfBinary/gobatch/batch"
)

// Channel is a Source that reads from input until it is closed.
// The input channel can be buffered or unbuffered.
type Channel struct {
	Input <-chan interface{}
}

// Read reads from items until the input channel is closed.
func (s *Channel) Read(ctx context.Context, ps* batch.PipelineStage) {
	defer ps.Close()

	out := ps.Output
	for item := range s.Input {
		out <- batch.NextItem(ps, item)
	}
}
