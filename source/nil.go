package source

import (
	"context"
	"time"

	"github.com/MasterOfBinary/gobatch/batch"
)

type nilSource struct {
	duration time.Duration
}

// Nil returns a Source that doesn't read any data. Instead it closes
// the channels after specified duration. It can be used as a mock Source.
func Nil(duration time.Duration) batch.Source {
	return &nilSource{
		duration: duration,
	}
}

// Read doesn't read anything.
func (s *nilSource) Read(ctx context.Context, in <-chan *batch.Item, items chan<- *batch.Item, errs chan<- error) {
	time.Sleep(s.duration)
	close(items)
	close(errs)
}
