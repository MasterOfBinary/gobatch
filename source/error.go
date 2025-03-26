package source

import (
	"context"

	"github.com/MasterOfBinary/gobatch/batch"
)

// Error is a Source that returns an error and then closes immediately.
// It can be used as a mock Source.
type Error[I any] struct {
	Err error
}

// Read returns an error and then closes.
func (s *Error[I]) Read(ctx context.Context, ps *batch.PipelineStage[I, I]) {
	ps.Errors <- s.Err
	ps.Close()
}
