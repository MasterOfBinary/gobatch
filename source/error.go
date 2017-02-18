package source

import (
	"context"

	"github.com/MasterOfBinary/gobatch/batch"
)

// Error is a Source that returns an error and then closes immediately.
// It can be used as a mock Source.
type Error struct {
	Err error
}

// Read returns an error and then closes.
func (s *Error) Read(ctx context.Context, ps batch.PipelineStage) {
	ps.Error() <- s.Err
	ps.Close()
}
