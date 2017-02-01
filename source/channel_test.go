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

	t.Run("read", func(t *testing.T) {
		var wg sync.WaitGroup

		ch := make(chan interface{}, size)
		defer close(ch)

		s := Channel(ch)

		wg.Add(1)
		go func() {
			defer wg.Done()
			values, err := s.Read(ctx)
			assert.Nil(t, err)
			assert.NotEmpty(t, values)
			assert.EqualValues(t, 1, values[0])
		}()

		ch <- 1
		wg.Wait()
	})

	t.Run("context done", func(t *testing.T) {
		var wg sync.WaitGroup

		ch := make(chan interface{}, size)
		defer close(ch)

		s := Channel(ch)

		ctx, cancel := context.WithCancel(ctx)

		wg.Add(1)
		go func() {
			defer wg.Done()
			values, err := s.Read(ctx)
			assert.Nil(t, err)
			assert.Empty(t, values)
		}()

		cancel()
		wg.Wait()
	})

	t.Run("close", func(t *testing.T) {
		var wg sync.WaitGroup

		ch := make(chan interface{}, size)

		s := Channel(ch)

		wg.Add(1)
		go func() {
			defer wg.Done()
			values, err := s.Read(ctx)
			assert.Nil(t, err)
			assert.Empty(t, values)
		}()

		close(ch)
		wg.Wait()
	})
}
