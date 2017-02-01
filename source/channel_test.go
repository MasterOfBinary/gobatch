package source

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChannelSource_Read(t *testing.T) {
	t.Parallel()

	size := 10
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	defer wg.Wait()

	itemsIn := make(chan interface{}, size)
	itemsOut := make(chan interface{})
	errsOut := make(chan error)

	s := Channel(itemsIn)

	wg.Add(1)
	go func() {
		defer wg.Done()
		s.Read(ctx, itemsOut, errsOut)
	}()

	itemsIn <- 0
	itemsIn <- 1
	itemsIn <- 2
	close(itemsIn)

	for i := 0; i < 5; i++ {
		select {
		case err, ok := <-errsOut:
			if ok {
				t.Error(err)
			}
		case item, ok := <-itemsOut:
			if ok {
				assert.Equal(t, i, item)
			} else {
				assert.FailNow(t, "item channel closed prematurely")
			}
		}
	}
}
