package processor

import (
	"context"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
)

type nilProcessor struct {
	duration time.Duration
}

// Nil returns a Processor that discards all data after a specified duration.
// It can be used as a mock Processor.
func Nil(duration time.Duration) batch.Processor {
	return &nilProcessor{
		duration: duration,
	}
}

// Process discards all data sent to it after a certain amount of time.
func (p *nilProcessor) Process(ctx context.Context, ps batch.PipelineStage) {
	time.Sleep(p.duration)
	ps.Close()
}
