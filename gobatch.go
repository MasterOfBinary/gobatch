package gobatch

import (
	"context"
	"time"

	"github.com/MasterOfBinary/gobatch/processor"
	"github.com/MasterOfBinary/gobatch/source"
)

type BatchConfig struct {
	MinTime  time.Duration
	MaxTime  time.Duration
	MinItems int64
	MaxItems int64
}

type Batch interface {
	Go(ctx context.Context, s source.Source, p processor.Processor) <-chan error
}
