package source

import (
	"context"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
)

// Nil is a Source that doesn't read any data. Instead it closes the
// pipeline stage after specified duration. It can be used as a mock
// Source.
type Nil[I any] struct {
	Duration time.Duration
}

// Read doesn't read anything.
func (s *Nil[I]) Read(ctx context.Context, ps *batch.PipelineStage[I, I]) {
	time.Sleep(s.Duration)
	ps.Close()
}
