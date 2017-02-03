package gobatch

import (
	"context"

	"github.com/MasterOfBinary/gobatch/processor"
	"github.com/MasterOfBinary/gobatch/source"
)

type Batch interface {
	Go(ctx context.Context, s source.Source, p processor.Processor) <-chan error

	Done() <-chan struct{}
}

func Must(b Batch, err error) Batch {
	if err != nil {
		panic(err)
	}
	return b
}

// go:generate mockery -name=Batch -inpkg -case=underscore
