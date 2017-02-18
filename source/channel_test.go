package source

import (
	"context"
	"sync"
	"testing"

	"github.com/MasterOfBinary/gobatch/batch"
)

func TestChannelSource_Read(t *testing.T) {
	t.Skip()

	size := 10
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	defer wg.Wait()

	in := make(chan interface{}, size)
	items := make(chan *batch.Item)
	_ = make(chan error)

	itemGen := batch.NewMockItemGenerator()
	defer itemGen.Close()

	s := Channel{
		Input: in,
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		s.Read(ctx, nil)
	}()

	numItems := 10
	for i := 0; i < numItems; i++ {
		in <- i
	}
	close(in)

	i := 0
	for item := range items {
		if i > numItems-1 {
			t.Fatalf("items in items > %v", i)
		}

		if item.Get() != i {
			t.Errorf("items <- %v, want %v", item, i)
		}

		i++
	}

	if i < numItems {
		t.Errorf("items in items < %v", i)
	}
}
