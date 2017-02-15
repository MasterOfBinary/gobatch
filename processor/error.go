package processor

import (
	"context"

	"github.com/MasterOfBinary/gobatch/batch"
)

type errorProcessor struct {
	err error
}

// Error returns a Processor that returns an error while processing.
func Error(err error) batch.Processor {
	return &errorProcessor{
		err: err,
	}
}

// Process discards all data sent to it after a certain amount of time.
func (p *errorProcessor) Process(ctx context.Context, items []*batch.Item, errs chan<- error) {
	errs <- p.err
	close(errs)
}
